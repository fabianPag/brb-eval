package transport

import (
	"brb-eval/internal/node/core"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const MaxMessageSize = 1024 * 1024 // 1 MiB

type PeerConn struct {
	conn net.Conn
	mu   sync.Mutex
}

type TCPTransport struct {
	id       core.NodeID
	peers    map[core.NodeID]string
	recvChan chan core.TransportMessage
	conns    map[core.NodeID]*PeerConn
	mu       sync.RWMutex
}

func NewTCPTransport(id core.NodeID) core.Transport {
	return &TCPTransport{
		id:       id,
		peers:    make(map[core.NodeID]string),
		recvChan: make(chan core.TransportMessage, 1000),
		conns:    make(map[core.NodeID]*PeerConn),
	}
}

func (t *TCPTransport) SetPeers(peers map[core.NodeID]string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peers = peers
}

func (t *TCPTransport) getOrDialConn(id core.NodeID) (*PeerConn, error) {
	t.mu.RLock()
	pc, exists := t.conns[id]
	t.mu.RUnlock()

	if exists {
		return pc, nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if pc, exists = t.conns[id]; exists {
		return pc, nil
	}

	addr, ok := t.peers[id]
	if !ok {
		return nil, fmt.Errorf("unknown peer %d", id)
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	pc = &PeerConn{conn: conn}
	t.conns[id] = pc

	return pc, nil
}

func (t *TCPTransport) Send(toID core.NodeID, payload []byte) error {
	pc, err := t.getOrDialConn(toID)
	if err != nil {
		return err
	}

	packet := make([]byte, 8+len(payload))

	binary.BigEndian.PutUint32(packet[0:4], uint32(t.id))
	binary.BigEndian.PutUint32(packet[4:8], uint32(len(payload)))

	copy(packet[8:], payload)

	pc.mu.Lock()
	_, writeErr := pc.conn.Write(packet)
	pc.mu.Unlock()

	if writeErr != nil {
		err := pc.conn.Close()
		if err != nil {
			return err
		}
		t.mu.Lock()
		delete(t.conns, toID)
		t.mu.Unlock()

		return fmt.Errorf("send failed: %w", writeErr)
	}

	return nil
}

func (t *TCPTransport) ConnectAll() error {
	t.mu.RLock()
	peers := make([]core.NodeID, 0, len(t.peers))
	for id := range t.peers {
		peers = append(peers, id)
	}
	t.mu.RUnlock()

	for _, id := range peers {
		if id == t.id {
			continue
		}
		for i := 0; i < 3; i++ {
			if _, err := t.getOrDialConn(id); err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
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
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {

		}
	}(conn)

	for {
		var senderID, length int32
		if err := binary.Read(conn, binary.BigEndian, &senderID); err != nil {
			return
		}
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			return
		}

		if length > MaxMessageSize || length < 0 {
			fmt.Printf("[ERROR] Message too large from node %d: %d bytes\n", senderID, length)
			return
		}

		buf := make([]byte, length)
		if _, err := io.ReadFull(conn, buf); err != nil {
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
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.peers)
}
