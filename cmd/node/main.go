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

type brbAlgorithm interface {
	HandleIncoming(core.NodeID, []byte)
	InitiateBroadcast([]byte)
}

func main() {
	nodeID := flag.Int("id", -1, "this node's ID")
	nodeAddr := flag.String("addr", "", "this nodes host:port")
	controllerAddr := flag.String("controller", "", "address of the controller")
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

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		fmt.Println("[ERROR] Failed to generate key pair:", err)
		os.Exit(1)
	}

	config, err := sendRegisterAndWait(*controllerAddr, *nodeAddr, *nodeID, pub)
	if err != nil {
		fmt.Printf("[ERROR] Failed to register with controller: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Registered with controller. Network has %d peers.\n", len(config.Peers))

	peers := make(map[core.NodeID]string)
	publicKeys := make(map[core.NodeID]ed25519.PublicKey)
	for id, info := range config.Peers {
		peers[core.NodeID(id)] = info.Address
		publicKeys[core.NodeID(id)] = info.PublicKey
	}

	tcpTransport.SetPeers(peers)

	fmt.Println("Connecting to peers...")
	if err := tcpTransport.ConnectAll(); err != nil {
		fmt.Printf("[ERROR] Failed to connect to peers: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Connected to all peers.")

	metricsInstance := metrics.New()

	mgr := manager.New(core.NodeID(*nodeID), tcpTransport, config.Role, metricsInstance)

	sentHashString := ""

	var brb1, brb2 brbAlgorithm

	var payload1, payload2 []byte

	isTwinsSender := config.Role == "twins_sender" && config.DesignatedSender == *nodeID
	if isTwinsSender {
		fmt.Println("Twins sender, splitting network")
		numPeers := mgr.NumPeers()
		partitionMid := core.NodeID(numPeers / 2)

		p1Manager := manager.NewSplitManager(mgr, 0, partitionMid)
		p2Manager := manager.NewSplitManager(mgr, partitionMid, core.NodeID(numPeers))
		selectedAlgorithm := strings.ToLower(config.Algorithm)
		switch selectedAlgorithm {
		case "bracha":
			brb1 = bracha.NewBracha(p1Manager, config.T)
			brb2 = bracha.NewBracha(p2Manager, config.T)
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
		payload1 = make([]byte, config.PayloadSize)
		rand.Read(payload1)

		payload2 = make([]byte, config.PayloadSize)
		rand.Read(payload2)
	} else {
		selectedAlgorithm := strings.ToLower(config.Algorithm)
		switch selectedAlgorithm {
		case "bracha":
			brb1 = bracha.NewBracha(mgr, config.T)
		case "afrt":
			brb1 = afrt.NewAFRT(mgr, config.T, config.D, priv, publicKeys)
		case "codedmbrb":
			brb1 = codedmbrb.NewCodedMBRB(mgr, config.T, config.D, config.DesignatedSender, config.PublicCryptoParams, config.PrivateCryptoParams)
		default:
			fmt.Printf("[ERROR] Unknown algorithm: %s. Available options: bracha, codedmbrb, afrt\n", selectedAlgorithm)
			os.Exit(1)
		}

		mgr.OnReceive(brb1.HandleIncoming)
		go mgr.Run()

		if config.DesignatedSender == *nodeID {
			payload1 = make([]byte, config.PayloadSize)
			rand.Read(payload1)

			hashBytes := sha256.Sum256(payload1)
			sentHashString = hex.EncodeToString(hashBytes[:])
		}
	}

	fmt.Printf("Signaling ready...\n")
	if err := sendReadyAndWait(*controllerAddr, *nodeID); err != nil {
		fmt.Printf("[ERROR] Failed to signal ready: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("All nodes ready. Starting %s. Acting as role %s\n", config.Algorithm, config.Role)

	if isTwinsSender {
		fmt.Printf("Initiating broadcast of two %d byte payloads.\n", config.PayloadSize)
		metricsInstance.RecordInitTime()
		brb1.InitiateBroadcast(payload1)
		brb2.InitiateBroadcast(payload2)
	} else if config.DesignatedSender == *nodeID {
		fmt.Printf("Initiating broadcast of %d byte payload.\n", config.PayloadSize)
		metricsInstance.RecordInitTime()
		brb1.InitiateBroadcast(payload1)
	}

	delivered := false
	var deliveredPayload []byte
	if config.Role == "honest" {
		deliveredPayload, delivered = mgr.WaitForDelivery(30 * time.Second) // Wait until manager signals delivery or timeout after 30 seconds. Optimized towards Shadow, feel free to change lol
	}

	fmt.Println("Signaling done...")
	if err := sendDoneAndWait(*controllerAddr, *nodeID); err != nil {
		fmt.Printf("[ERROR] Failed to signal done: %v\n", err)
	}
	fmt.Println("All nodes done. Sending metrics...")

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

	if err := sendMetricsAndWait(*controllerAddr, metricsData); err != nil {
		fmt.Printf("[ERROR] Failed to send metrics: %v\n", err)
	}
	fmt.Printf("Metrics sent successfully.\n")

	metricsInstance.PrintMetrics()

	os.Exit(0)
}

func sendMetricsAndWait(controllerAddr string, data api.MetricsData) error {
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
	return nil
}

func sendRegisterAndWait(controllerAddr string, nodeAddr string, nodeID int, publicKey ed25519.PublicKey) (api.Config, error) {
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

func sendReadyAndWait(controllerURL string, id int) error {
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

func sendDoneAndWait(controllerURL string, id int) error {
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
