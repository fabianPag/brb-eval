package main

import (
	"brb-eval/internal/controller/api"
	"brb-eval/internal/node/algorithm"
	"brb-eval/internal/node/core"
	"brb-eval/internal/node/fault"
	"brb-eval/internal/node/manager"
	"brb-eval/internal/node/transport"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func parseDroppedNodes(rawDrop string) map[core.NodeID]bool {
	dropped := make(map[core.NodeID]bool)
	if rawDrop == "" {
		return dropped
	}
	parts := strings.Split(rawDrop, ",")
	for _, p := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(p))
		if err == nil {
			dropped[core.NodeID(id)] = true
		}
	}
	return dropped
}

func main() {
	nodeID := flag.Int("id", -1, "this node's ID")
	nodeAddr := flag.String("addr", "", "this nodes host:port")
	dropNodes := flag.String("drop", "", "comma-separated list of node IDs to silently drop (e.g., '1,3')")
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

	config, err := registerWithController(*controllerAddr, *nodeAddr, *nodeID)
	if err != nil {
		fmt.Printf("[ERROR] Failed to register with controller: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Registered with controller. Network has %d peers.\n", len(config.Peers))

	peers := make(map[core.NodeID]string)
	for id, info := range config.Peers {
		peers[core.NodeID(id)] = info.Address
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
	fmt.Printf("Received start signal. Starting algorithm.\n")

	droppedMap := parseDroppedNodes(*dropNodes)
	var injector fault.Injector
	if len(droppedMap) > 0 {
		fmt.Printf("[INFO] Simulating silent churn. Dropping messages to nodes: %v\n", *dropNodes)
		injector = fault.NewChurnInjector(droppedMap)
	} else {
		injector = &fault.PassThroughInjector{}
	}

	mgr := manager.New(core.NodeID(*nodeID), tcpTransport, injector)

	bracha := algorithm.NewBracha(mgr)

	mgr.OnReceive(bracha.HandleIncoming)

	if config.ShouldInitiate {
		fmt.Printf("Initiating broadcast.\n")
		bracha.Broadcast("Hello")
	}

	mgr.Run()
}

func registerWithController(controllerAddr string, nodeAddr string, nodeID int) (api.Config, error) {
	myInfo := api.PeerInfo{
		ID:        nodeID,
		Address:   nodeAddr,
		PublicKey: nil,
	}

	body, err := json.Marshal(myInfo)
	if err != nil {
		return api.Config{}, err
	}

	resp, err := http.Post("http://"+controllerAddr+"/register", "application/json", bytes.NewReader(body))
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

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
