package core

import "time"

type NodeID int32

type Manager interface {
	Broadcast(msg []byte)
	SendBatch(messages map[NodeID][]byte)
	OnReceive(handler MessageHandler)
	ID() NodeID
	NumPeers() int
	Deliver(initiator NodeID, value []byte)
	Run()
	WaitForDelivery(timeout time.Duration) ([]byte, bool)
}

type TransportMessage struct {
	From    NodeID
	Payload []byte
}

type MessageHandler func(from NodeID, payload []byte)

type Transport interface {
	Receive() <-chan TransportMessage
	Send(to NodeID, payload []byte) error
	Listen(addr string) error
	ConnectAll() error
	NumPeers() int
	SetPeers(peers map[NodeID]string)
	GetRTTs() map[NodeID]time.Duration
}

type Metrics interface {
	RecordInitTime()
	RecordDelivery() int64
	RecordMessageSent(payloadSize int)
	RecordMessageReceived(payloadSize int)
	PrintMetrics()
}
