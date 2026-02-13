package node

import (
	"brb-eval/core"
	"fmt"
)

type Node struct {
	id        core.NodeID
	transport core.Transport
	handler   core.MessageHandler
}

func New(id core.NodeID, transport core.Transport) core.Node {
	return &Node{id: id, transport: transport}
}

func (n *Node) ID() core.NodeID {
	return n.id
}

func (n *Node) NumPeers() int {
	return n.transport.NumPeers()
}

func (n *Node) OnReceive(handler core.MessageHandler) {
	n.handler = handler
}

func (n *Node) Run() {
	for msg := range n.transport.Receive() {
		if n.handler != nil {
			n.handler(msg.From, msg.Payload)
		}
	}
}

func (n *Node) Send(to core.NodeID, payload []byte) {
	err := n.transport.Send(to, payload)
	if err != nil {
		fmt.Printf("[ERROR] Failed to send to node %d: %v\n", to, err)
	}
}

func (n *Node) Broadcast(payload []byte) {
	for i := 0; i < n.transport.NumPeers(); i++ {
		peerID := core.NodeID(i)
		if peerID == n.id {
			continue
		}
		if err := n.Send(peerID, payload); err != nil {
			fmt.Printf("[ERROR] Failed to send to node %d: %v\n", peerID, err)
		}
	}
}

func (n *Node) Deliver(initiator core.NodeID, value []byte) {
	fmt.Printf("[DELIVER] From node %d: %s\n", initiator, string(value))
}
