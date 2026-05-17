package manager

import (
	"brb-eval/internal/node/core"
	"fmt"
	"math/rand"
	"time"
)

type Manager struct {
	id          core.NodeID
	transport   core.Transport
	role        string
	numMsgDrops int
	honestNodes map[core.NodeID]bool
	handler     core.MessageHandler
	metrics     core.Metrics
	done        chan []byte
}

func New(id core.NodeID, transport core.Transport, role string, numMsgDrops int, honestNodes []int, metrics core.Metrics) *Manager {
	honestMap := make(map[core.NodeID]bool)
	for _, nodeID := range honestNodes {
		honestMap[core.NodeID(nodeID)] = true
	}
	return &Manager{
		id:          id,
		transport:   transport,
		role:        role,
		numMsgDrops: numMsgDrops,
		honestNodes: honestMap,
		metrics:     metrics,
		done:        make(chan []byte, 1),
	}
}

func (m *Manager) ID() core.NodeID {
	return m.id
}

func (m *Manager) NumPeers() int {
	return m.transport.NumPeers()
}

func (m *Manager) OnReceive(handler core.MessageHandler) {
	m.handler = handler
}

func (m *Manager) Run() {
	for msg := range m.transport.Receive() {
		if m.role == "silent" {
			continue
		}

		if m.handler != nil {
			m.metrics.RecordMessageReceived(len(msg.Payload))
			go m.handler(msg.From, msg.Payload)
		}
	}
}

func (m *Manager) selectDroppedNodes(honestTargets []core.NodeID) map[core.NodeID]bool {
	droppedSet := make(map[core.NodeID]bool)
	drops := min(m.numMsgDrops, len(honestTargets))

	if drops > 0 {
		perm := rand.Perm(len(honestTargets))
		for i := range drops {
			droppedSet[honestTargets[perm[i]]] = true
		}
	}

	return droppedSet
}

func (m *Manager) SendBatch(messages map[core.NodeID][]byte) {
	if m.role == "silent" {
		return
	}

	if payload, ok := messages[m.id]; ok {
		go m.handler(m.id, payload)
	}

	var honestTargets []core.NodeID
	for to := range messages {
		if to != m.id && m.honestNodes[to] {
			honestTargets = append(honestTargets, to)
		}
	}

	droppedSet := m.selectDroppedNodes(honestTargets)

	for to, payload := range messages {
		if to == m.id || droppedSet[to] {
			continue
		}

		go func(target core.NodeID, data []byte) {
			err := m.transport.Send(target, data)
			if err != nil {
				fmt.Printf("[ERROR] Failed to send to node %d: %v\n", target, err)
				return
			}

			m.metrics.RecordMessageSent(len(data))
		}(to, payload)
	}
}

func (m *Manager) Broadcast(payload []byte) {
	messages := make(map[core.NodeID][]byte)
	for i := 0; i < m.transport.NumPeers(); i++ {
		messages[core.NodeID(i)] = payload
	}
	m.SendBatch(messages)
}

func (m *Manager) Deliver(initiator core.NodeID, payload []byte) {
	count := m.metrics.RecordDelivery()

	if count > 1 {
		fmt.Printf("Property violation! Deliver function called multiple times!\n")
		return
	}

	fmt.Printf("[DELIVER] From node %d\n", initiator)
	m.done <- payload
}

func (m *Manager) WaitForDelivery(timeout time.Duration) ([]byte, bool) {
	select {
	case payload := <-m.done:
		return payload, true
	case <-time.After(timeout):
		fmt.Println("Timed out waiting for delivery")
		return nil, false
	}
}
