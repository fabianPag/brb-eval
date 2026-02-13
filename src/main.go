package main

import (
	"brb-eval/algorithm"
	"brb-eval/core"
	"brb-eval/node"
	"brb-eval/transport"
	"flag"
	"fmt"
	"os"
	"sort"
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

func main() {
	nodeAddr := flag.String("addr", "", "this nodes host:port")
	rawPeers := flag.String("peers", "", "all peers host:port")
	inputValue := flag.String("inputValue", "", "value to broadcast")
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

	node := node.New(nodeID, tcpTransport)
	bracha := algorithm.NewBracha(node)

	node.OnReceive(bracha.HandleIncoming) // Register handler

	if *inputValue != "" {
		bracha.Broadcast(*inputValue)
	}
	
	node.Run() // Start the receive loop
}
