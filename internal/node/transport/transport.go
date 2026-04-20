package transport

import (
	"brb-eval/internal/node/core"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

const MaxMessageSize = 10 * 1024 * 1024 // 10 MiB

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
		recvChan: make(chan core.TransportMessage),
		conns:    make(map[core.NodeID]*PeerConn),
	}
}

func (t *TCPTransport) SetPeers(peers map[core.NodeID]string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peers = peers
}

func (t *TCPTransport) dialConn(id core.NodeID) {
	addr, ok := t.peers[id]
	if !ok {
		// If we don't know the ip, then abort.
		return
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Printf("[ERROR] Failed to dial %d: %v\n", id, err)
		return
	}

	handshake := make([]byte, 4)
	binary.BigEndian.PutUint32(handshake, uint32(t.id))
	if _, err := conn.Write(handshake); err != nil {
		conn.Close()
		return
	}

	t.registerConn(id, conn)
	go t.handleIncoming(conn, id)
}

func (t *TCPTransport) Send(toID core.NodeID, payload []byte) error {
	t.mu.RLock()
	pc, exists := t.conns[toID]
	t.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no active connection to peer %d", toID)
	}

	packet := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(packet[0:4], uint32(len(payload)))
	copy(packet[4:], payload)

	pc.mu.Lock()
	defer pc.mu.Unlock()
	_, err := pc.conn.Write(packet)

	return err
}

func (t *TCPTransport) ConnectAll() error {
	var wg sync.WaitGroup

	const maxWorkers = 20
	sem := make(chan struct{}, maxWorkers)

	for id := range t.peers {
		if id > t.id {
			wg.Add(1)

			sem <- struct{}{}

			go func(peerID core.NodeID) {
				defer wg.Done()

				defer func() { <-sem }()

				t.dialConn(peerID)
			}(id)
		}
	}

	wg.Wait() // Wait for all connections to be dialed so we don't report ready before
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

			go t.acceptHandshake(conn)
		}
	}()
	return nil
}

func (t *TCPTransport) acceptHandshake(conn net.Conn) {
	var dialerID uint32
	if err := binary.Read(conn, binary.BigEndian, &dialerID); err != nil {
		conn.Close()
		return
	}

	peerID := core.NodeID(dialerID)

	t.registerConn(peerID, conn)
	go t.handleIncoming(conn, peerID)
}

func (t *TCPTransport) registerConn(id core.NodeID, conn net.Conn) {
	t.mu.Lock()
	t.conns[id] = &PeerConn{conn: conn}
	t.mu.Unlock()
}

func (t *TCPTransport) handleIncoming(conn net.Conn, peerID core.NodeID) {
	defer func() {
		conn.Close()
		t.mu.Lock()
		delete(t.conns, peerID)
		t.mu.Unlock()
	}()

	for {
		var length int32
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			return
		}

		if length > MaxMessageSize || length < 0 {
			fmt.Println("Received message outside allowed size:", length)
			return
		}

		buf := make([]byte, length)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}

		t.recvChan <- core.TransportMessage{
			From:    peerID,
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
