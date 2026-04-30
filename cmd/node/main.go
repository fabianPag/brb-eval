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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
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
		fmt.Println("[ERROR] Failed to generate key pair:", err)
		os.Exit(1)
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

	sentHashString := ""

	type brbAlgorithm interface {
		HandleIncoming(core.NodeID, []byte)
		InitiateBroadcast([]byte)
	}

	var brb brbAlgorithm
	isTwinsSender := config.Role == "twins_sender" && config.DesignatedSender == *nodeID
	if isTwinsSender {
		fmt.Println("Twins sender, splitting network")
		numPeers := mgr.NumPeers()
		partitionMid := core.NodeID(numPeers / 2)

		p1Manager := manager.NewSplitManager(mgr, 0, partitionMid)
		p2Manager := manager.NewSplitManager(mgr, partitionMid, core.NodeID(numPeers))
		var brb1, brb2 brbAlgorithm
		selectedAlgorithm := strings.ToLower(config.Algorithm)
		switch selectedAlgorithm {
		case "bracha":
			brb1 = bracha.NewBracha(p1Manager)
			brb2 = bracha.NewBracha(p2Manager)
		case "afrt":
			brb1 = afrt.NewAFRT(p1Manager, config.T, config.D, priv, publicKeys)
			brb2 = afrt.NewAFRT(p2Manager, config.T, config.D, priv, publicKeys)
		case "codedmbrb":
			brb1 = codedmbrb.NewCodedMBRB(p1Manager, config.T, config.D, config.DesignatedSender, config.PublicCryptoParams, config.PrivateCryptoParams)
			brb2 = codedmbrb.NewCodedMBRB(p2Manager, config.T, config.D, config.DesignatedSender, config.PublicCryptoParams, config.PrivateCryptoParams)
		default:
			fmt.Printf("[ERROR] Unknown algorithm: %s. Available options: bracha, codedmbrb, afrt\n", selectedAlgorithm)
			os.Exit(1)
		}
		mgr.OnReceive(func(from core.NodeID, payload []byte) {
			if from >= partitionMid {
				go brb2.HandleIncoming(from, payload)
			} else {
				go brb1.HandleIncoming(from, payload)
			}
		})
		go mgr.Run()
		payload1 := make([]byte, config.PayloadSize)
		rand.Read(payload1)

		payload2 := make([]byte, config.PayloadSize)
		rand.Read(payload2)
		metricsInstance.RecordInitTime()

		brb1.InitiateBroadcast(payload1)
		brb2.InitiateBroadcast(payload2)
	} else {
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

			hashBytes := sha256.Sum256(payload)
			sentHashString = hex.EncodeToString(hashBytes[:])

			fmt.Printf("Initiating broadcast of %d byte payload.\n", config.PayloadSize)
			metricsInstance.RecordInitTime()

			brb.InitiateBroadcast(payload)
		}
	}

	delivered := false
	var deliveredPayload []byte
	if config.Role == "honest" {
		deliveredPayload, delivered = mgr.WaitForDelivery(30 * time.Second) // Wait until manager signals delivery or timeout after 30 seconds. Optimized towards Shadow, feel free to change lol
	}

	fmt.Println("[INFO] Signaling done. Waiting for controller to signal other nodes are done...")
	if err := signalDoneAndWait(*controllerAddr, *nodeID); err != nil {
		fmt.Printf("[ERROR] Failed to wait for network delivered signal: %v\n", err)
	}

	deliveredHashString := ""
	if delivered && deliveredPayload != nil {
		hashBytes := sha256.Sum256(deliveredPayload)
		deliveredHashString = hex.EncodeToString(hashBytes[:])
	}

	metricsResults := metricsInstance.GetMetrics()

	metricsData := api.MetricsData{
		NodeID:           *nodeID,
		Role:             config.Role,
		Delivered:        delivered,
		DeliveredHash:    deliveredHashString,
		SentHash:         sentHashString,
		FirstMessageTime: metricsResults.FirstMessageTime,
		DeliveryTime:     metricsResults.DeliveryTime,
		MessagesReceived: metricsResults.MessagesReceived,
		BytesReceived:    metricsResults.BytesReceived,
		MessagesSent:     metricsResults.MessagesSent,
		BytesSent:        metricsResults.BytesSent,
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
