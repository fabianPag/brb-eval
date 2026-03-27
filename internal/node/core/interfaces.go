package core

type NodeID int32

type Manager interface {
	Send(to NodeID, msg []byte)
	Broadcast(msg []byte)
	OnReceive(handler MessageHandler)
	ID() NodeID
	NumPeers() int
	Deliver(initiator NodeID, value []byte)
	Run()
	Wait()
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
}

type Metrics interface {
	RecordInitTime()
	RecordDelivery()
	RecordMessageSent(payloadSize int)
	RecordMessageRecieved(payloadSize int)
	PrintMetrics()
}
