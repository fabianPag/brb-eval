package core

type NodeID int32

type Node interface {
	Send(to NodeID, msg []byte)
	Broadcast(msg []byte)
	OnReceive(handler MessageHandler)
	ID() NodeID
	NumPeers() int
	Deliver(initiator NodeID, value []byte)
	Run()
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
	NumPeers() int
}
