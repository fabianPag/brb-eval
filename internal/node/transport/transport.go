package transport

import (
	"brb-eval/internal/node/core"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

const MaxMessageSize = 1024 * 1024 // 1 MiB

type TCPTransport struct {
	id       core.NodeID
	peers    map[core.NodeID]string
	recvChan chan core.TransportMessage
	conns    map[core.NodeID]net.Conn
	mu       sync.Mutex
}

func NewTCPTransport(id core.NodeID) core.Transport {
	return &TCPTransport{
		id:       id,
		peers:    make(map[core.NodeID]string),
		recvChan: make(chan core.TransportMessage, 1000),
		conns:    make(map[core.NodeID]net.Conn),
	}
}

func (t *TCPTransport) SetPeers(peers map[core.NodeID]string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peers = peers
}

func (t *TCPTransport) getOrDialConn(toID core.NodeID) (net.Conn, error) {
	t.mu.Lock()
	conn, exists := t.conns[toID]
	t.mu.Unlock()

	if exists {
		return conn, nil
	}

	t.mu.Lock()
	addr, ok := t.peers[toID]
	t.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("unknown peer %d", toID)
	}

	newConn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.conns[toID] = newConn
	t.mu.Unlock()
	return newConn, nil
}

func (t *TCPTransport) ConnectAll() error {
	t.mu.Lock()
	peers := make([]core.NodeID, 0, len(t.peers))
	for id := range t.peers {
		peers = append(peers, id)
	}
	t.mu.Unlock()

	for _, id := range peers {
		if id == t.id {
			continue
		}
		if _, err := t.getOrDialConn(id); err != nil {
			return fmt.Errorf("failed to connect to node %d: %v", id, err)
		}
	}
	return nil
}

func (t *TCPTransport) Send(toID core.NodeID, payload []byte) error {
	conn, err := t.getOrDialConn(toID)
	if err != nil {
		return err
	}

	senderID := int32(t.id)
	if err := binary.Write(conn, binary.BigEndian, senderID); err != nil {
		return t.handleWriteError(toID, conn, err)
	}

	if err := binary.Write(conn, binary.BigEndian, int32(len(payload))); err != nil {
		return t.handleWriteError(toID, conn, err)
	}
	_, err = conn.Write(payload)
	if err != nil {
		return t.handleWriteError(toID, conn, err)
	}
	return nil
}

func (t *TCPTransport) handleWriteError(toID core.NodeID, conn net.Conn, err error) error {
	conn.Close()
	t.mu.Lock()
	delete(t.conns, toID)
	t.mu.Unlock()
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

	for {
		var senderID, length int32
		if err := binary.Read(conn, binary.BigEndian, &senderID); err != nil {
			fmt.Printf("[ERROR] Failed to read sender ID: %v\n", err)
			return
		}
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			fmt.Printf("[ERROR] Failed to read message length: %v\n", err)
			return
		}

		if length > MaxMessageSize {
			fmt.Printf("[ERROR] Message too large from node %d: %d bytes\n", senderID, length)
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
}

func (t *TCPTransport) Receive() <-chan core.TransportMessage {
	return t.recvChan
}

func (t *TCPTransport) NumPeers() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.peers)
}
