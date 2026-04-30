package manager

import (
	"brb-eval/internal/node/core"
	"fmt"
	"time"
)

type Manager struct {
	id        core.NodeID
	transport core.Transport
	role      string
	handler   core.MessageHandler
	metrics   core.Metrics
	done      chan []byte
}

func New(id core.NodeID, transport core.Transport, role string, metrics core.Metrics) *Manager {
	return &Manager{
		id:        id,
		transport: transport,
		role:      role,
		metrics:   metrics,
		done:      make(chan []byte, 1),
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
		if m.role == "silent" || m.role == "isolated" {
			continue
		}

		if m.handler != nil {
			m.metrics.RecordMessageReceived(len(msg.Payload))
			go m.handler(msg.From, msg.Payload)
		}
	}
}

func (m *Manager) Send(to core.NodeID, payload []byte) {
	if m.role == "silent" {
		return
	}

	if to == m.id {
		go m.handler(m.id, payload)
		return
	}
	err := m.transport.Send(to, payload)
	if err != nil {
		fmt.Printf("[ERROR] Failed to send to node %d: %v\n", to, err)
		return
	}

	m.metrics.RecordMessageSent(len(payload))
}

func (m *Manager) Broadcast(payload []byte) {
	for i := 0; i < m.transport.NumPeers(); i++ {
		peerID := core.NodeID(i)
		go m.Send(peerID, payload)
	}
}

func (m *Manager) Deliver(initiator core.NodeID, payload []byte) {
	m.metrics.RecordDelivery()
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
