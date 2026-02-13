package transport

import (
	"brb-eval/core"
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

func NewTCPTransport(id core.NodeID, peers map[core.NodeID]string) core.Transport {
	return &TCPTransport{
		id:       id,
		peers:    peers,
		recvChan: make(chan core.TransportMessage, 1000),
	}
}

type TCPTransport struct {
	id       core.NodeID
	peers    map[core.NodeID]string
	recvChan chan core.TransportMessage
}

func (t *TCPTransport) Send(toID core.NodeID, payload []byte) error {
	addr := t.peers[toID]
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	senderID := int32(t.id)
	if err := binary.Write(conn, binary.BigEndian, senderID); err != nil {
		return err
	}

	if err := binary.Write(conn, binary.BigEndian, int32(len(payload))); err != nil {
		return err
	}
	_, err = conn.Write(payload)
	return err
}

func (t *TCPTransport) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	fmt.Printf("[INFO] Started transport at %s\n", addr)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				continue
			}
			go t.handleIncoming(conn)
		}
	}()
	return nil
}

func (t *TCPTransport) handleIncoming(conn net.Conn) {
	defer conn.Close()

	var senderID, length int32
	if err := binary.Read(conn, binary.BigEndian, &senderID); err != nil {
		fmt.Printf("[ERROR] Failed to read sender ID: %v\n", err)
		return
	}
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		fmt.Printf("[ERROR] Failed to read message length: %v\n", err)
		return
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(conn, buf); err != nil {
		fmt.Printf("[ERROR] Failed to read message payload: %v\n", err)
		return
	}

	t.recvChan <- core.TransportMessage{
		From:    core.NodeID(senderID),
		Payload: buf,
	}
}

func (t *TCPTransport) Receive() <-chan core.TransportMessage {
	return t.recvChan
}

func (t *TCPTransport) NumPeers() int {
	return len(t.peers)
}
