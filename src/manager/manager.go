package manager

import (
	"brb-eval/core"
	"brb-eval/fault"
	"fmt"
)

type Manager struct {
	id        core.NodeID
	transport core.Transport
	injector  fault.Injector
	handler   core.MessageHandler
}

func New(id core.NodeID, transport core.Transport, injector fault.Injector) *Manager {
	if injector == nil {
		injector = &fault.PassThroughInjector{}
	}
	return &Manager{
		id:        id,
		transport: transport,
		injector:  injector,
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
		if m.handler != nil {
			m.handler(msg.From, msg.Payload)
		}
	}
}

func (m *Manager) Send(to core.NodeID, payload []byte) {
	err := m.injector.ProcessSend(to, payload, m.transport)
	if err != nil {
		fmt.Printf("[ERROR] Failed to send to node %d: %v\n", to, err)
	}
}

func (m *Manager) Broadcast(payload []byte) {
	for i := 0; i < m.transport.NumPeers(); i++ {
		peerID := core.NodeID(i)
		if peerID == m.id {
			continue
		}
		m.Send(peerID, payload)
	}
}

func (m *Manager) Deliver(initiator core.NodeID, value []byte) {
	fmt.Printf("[DELIVER] From node %d: %s\n", initiator, string(value))
}
