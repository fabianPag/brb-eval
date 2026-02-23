package main

import (
	"brb-eval/algorithm"
	"brb-eval/core"
	"brb-eval/fault"
	"brb-eval/manager"
	"brb-eval/transport"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func mapPeers(rawPeers string, nodeAddr string) (core.NodeID, map[core.NodeID]string) {
	addrList := strings.Split(rawPeers, ",")
	sort.Strings(addrList)
	peers := make(map[core.NodeID]string)
	nodeIndex := -1
	for id, addr := range addrList {
		peers[core.NodeID(id)] = addr
		if addr == nodeAddr {
			nodeIndex = id
		}
	}
	return core.NodeID(nodeIndex), peers
}

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
	nodeAddr := flag.String("addr", "", "this nodes host:port")
	rawPeers := flag.String("peers", "", "all peers host:port")
	inputValue := flag.String("inputValue", "", "value to broadcast")
	dropNodes := flag.String("drop", "", "comma-separated list of node IDs to silently drop (e.g., '1,3')")
	flag.Parse()

	nodeID, peers := mapPeers(*rawPeers, *nodeAddr)
	if nodeID < 0 {
		fmt.Printf("[ERROR] Address not found in peers list: %s\n", *nodeAddr)
		os.Exit(1)
	}

	tcpTransport := transport.NewTCPTransport(nodeID, peers)
	if err := tcpTransport.Listen(*nodeAddr); err != nil {
		fmt.Printf("[ERROR] Failed to start listener: %v\n", err)
		os.Exit(1)
	}

	droppedMap := parseDroppedNodes(*dropNodes)
	var injector fault.Injector
	if len(droppedMap) > 0 {
		fmt.Printf("[INFO] Simulating silent churn. Dropping messages to nodes: %v\n", *dropNodes)
		injector = fault.NewChurnInjector(droppedMap)
	} else {
		injector = &fault.PassThroughInjector{}
	}

	mgr := manager.New(nodeID, tcpTransport, injector)

	bracha := algorithm.NewBracha(mgr)

	mgr.OnReceive(bracha.HandleIncoming)

	if *inputValue != "" {
		bracha.Broadcast(*inputValue)
	}

	mgr.Run()
}