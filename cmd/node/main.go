package main

import (
	"brb-eval/internal/controller/api"
	"brb-eval/internal/node/algorithm/afrt"
	"brb-eval/internal/node/algorithm/bracha"
	"brb-eval/internal/node/algorithm/codedmbrb"
	"brb-eval/internal/node/core"
	"brb-eval/internal/node/manager"
	"brb-eval/internal/node/metrics"
	"brb-eval/internal/node/transport"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	nodeID := flag.Int("id", -1, "this node's ID")
	nodeAddr := flag.String("addr", "", "this nodes host:port")
	controllerAddr := flag.String("controller", "", "address of the controller")

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Fatal(err)
	}

	flag.Parse()

	if *nodeID < 0 || *nodeAddr == "" || *controllerAddr == "" {
		fmt.Println("Usage: node -id=<id> -addr=<host:port> -controller=<host:port>")
		os.Exit(1)
	}

	fmt.Printf("Starting node at %s\n", *nodeAddr)

	tcpTransport := transport.NewTCPTransport(core.NodeID(*nodeID))
	if err := tcpTransport.Listen(*nodeAddr); err != nil {
		fmt.Printf("[ERROR] Failed to start listener: %v\n", err)
		os.Exit(1)
	}

	config, err := registerWithController(*controllerAddr, *nodeAddr, *nodeID, pub)
	if err != nil {
		fmt.Printf("[ERROR] Failed to register with controller: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Registered with controller. Network has %d peers.\n", len(config.Peers))

	peers := make(map[core.NodeID]string)
	for id, info := range config.Peers {
		peers[core.NodeID(id)] = info.Address
	}

	publicKeys := make(map[core.NodeID]ed25519.PublicKey)
	for id, info := range config.Peers {
		publicKeys[core.NodeID(id)] = info.PublicKey
	}

	tcpTransport.SetPeers(peers)

	fmt.Println("Connecting to peers...")
	if err := tcpTransport.ConnectAll(); err != nil {
		fmt.Printf("[ERROR] Failed to connect to peers: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Connected to all peers.")

	fmt.Printf("Waiting for start signal...\n")
	if err := waitForStart(*controllerAddr, *nodeID); err != nil {
		fmt.Printf("[ERROR] Failed to wait for start signal: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Received start signal. Starting %s.\n", config.Algorithm)

	metricsInstance := metrics.New()

	fmt.Println("Acting as role:", config.Role)

	mgr := manager.New(core.NodeID(*nodeID), tcpTransport, config.Role, metricsInstance)

	type brbAlgorithm interface {
		HandleIncoming(core.NodeID, []byte)
		InitiateBroadcast([]byte)
	}

	var brb brbAlgorithm
	selectedAlgorithm := strings.ToLower(config.Algorithm)
	switch selectedAlgorithm {
	case "bracha":
		brb = bracha.NewBracha(mgr)
	case "afrt":
		brb = afrt.NewAFRT(mgr, config.T, config.D, priv, publicKeys)
	case "codedmbrb":
		brb = codedmbrb.NewCodedMBRB(mgr, config.T, config.D, config.DesignatedSender, config.PublicCryptoParams, config.PrivateCryptoParams)
	default:
		fmt.Printf("[ERROR] Unknown algorithm: %s. Available options: bracha, codedmbrb, afrt\n", selectedAlgorithm)
		os.Exit(1)
	}

	mgr.OnReceive(brb.HandleIncoming)
	go mgr.Run()

	if config.DesignatedSender == *nodeID {
		payload := make([]byte, config.PayloadSize)
		rand.Read(payload)

		fmt.Printf("Initiating broadcast of %d byte payload.\n", config.PayloadSize)
		metricsInstance.RecordInitTime()

		brb.InitiateBroadcast(payload)
	}

	if config.Role == "honest" {
		mgr.Wait() // Wait until manager signals delivery.
	}

	fmt.Println("[INFO] Signaling done. Waiting for controller to signal other nodes are done...")
	if err := signalDoneAndWait(*controllerAddr, *nodeID); err != nil {
		fmt.Printf("[ERROR] Failed to wait for network delivered signal: %v\n", err)
	}

	metricsResults := metricsInstance.GetMetrics()

	metricsData := api.MetricsData{
		NodeID:            *nodeID,
		Role:              config.Role,
		FirstMessageTime:  metricsResults.FirstMessageTime,
		DeliveryTime:      metricsResults.DeliveryTime,
		TotalMessagesRecv: metricsResults.TotalMessagesRecv,
		TotalBytesRecv:    metricsResults.TotalBytesRecv,
		TotalMessagesSent: metricsResults.TotalMessagesSent,
		TotalBytesSent:    metricsResults.TotalBytesSent,
	}

	if err := sendMetrics(*controllerAddr, metricsData); err != nil {
		fmt.Printf("[ERROR] Failed to send metrics: %v\n", err)
	}

	metricsInstance.PrintMetrics()

	os.Exit(0)
}

func sendMetrics(controllerAddr string, data api.MetricsData) error {
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: time.Second * 10,
	}
	resp, err := client.Post("http://"+controllerAddr+"/metrics", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	fmt.Printf("Metrics sent successfully.\n")
	return nil
}

func registerWithController(controllerAddr string, nodeAddr string, nodeID int, publicKey ed25519.PublicKey) (api.Config, error) {
	myInfo := api.PeerInfo{
		ID:        nodeID,
		Address:   nodeAddr,
		PublicKey: publicKey,
	}

	body, err := json.Marshal(myInfo)
	if err != nil {
		return api.Config{}, err
	}

	client := &http.Client{
		Timeout: time.Minute,
	}
	resp, err := client.Post("http://"+controllerAddr+"/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return api.Config{}, err
	}
	defer resp.Body.Close()

	var config api.Config
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return api.Config{}, err
	}

	return config, nil
}

func waitForStart(controllerURL string, id int) error {
	url := fmt.Sprintf("http://%s/ready?id=%d", controllerURL, id)

	client := &http.Client{
		Timeout: time.Minute,
	}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func signalDoneAndWait(controllerURL string, id int) error {
	url := fmt.Sprintf("http://%s/done?id=%d", controllerURL, id)

	client := &http.Client{
		Timeout: 0,
	}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
