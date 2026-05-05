/*
   Implementation of Raynal's version of Bracha's algorithm from the book "Fault-Tolerant Message-Passing Distributed Systems"
   DOI: 10.1007/978-3-319-94141-7
   https://link.springer.com/book/10.1007/978-3-319-94141-7
*/

package bracha

import (
	"brb-eval/internal/node/core"
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"sync"
)

type MsgType string

const (
	Init  MsgType = "INIT"
	Echo  MsgType = "ECHO"
	Ready MsgType = "READY"
)

type Message struct {
	Type      MsgType
	Sender    core.NodeID
	Initiator core.NodeID
	Value     []byte
}

type ValueHash [32]byte

type Bracha struct {
	mu      sync.Mutex
	manager core.Manager
	n       int
	t       int
	// Map by initiator and value, so we know exactly which values have been voted for.
	echoReceived  map[core.NodeID]map[ValueHash]map[core.NodeID]bool
	readyReceived map[core.NodeID]map[ValueHash]map[core.NodeID]bool
	// Map by initiator only, so we don't send multiple for different values from same initiator (equivocation attack).
	echoSent  map[core.NodeID]bool
	readySent map[core.NodeID]bool
	delivered map[core.NodeID]bool
}

func NewBracha(m core.Manager, t int) *Bracha {
	n := m.NumPeers()

	if n <= 3*t {
		panic(fmt.Sprintf("invalid parameters: require n > 3t, got n=%d t=%d", n, t))
	}

	return &Bracha{
		manager:       m,
		n:             n,
		t:             t,
		echoReceived:  make(map[core.NodeID]map[ValueHash]map[core.NodeID]bool),
		readyReceived: make(map[core.NodeID]map[ValueHash]map[core.NodeID]bool),
		echoSent:      make(map[core.NodeID]bool),
		readySent:     make(map[core.NodeID]bool),
		delivered:     make(map[core.NodeID]bool),
	}
}

func (b *Bracha) HandleIncoming(fromID core.NodeID, payload []byte) {
	var msg Message
	buf := bytes.NewReader(payload)
	if err := gob.NewDecoder(buf).Decode(&msg); err != nil {
		fmt.Printf("[ERROR] Failed to decode message from node %d: %v\n", fromID, err)
		return
	}
	msg.Sender = fromID
	initID := msg.Initiator

	b.mu.Lock()
    if b.delivered[initID] && b.readySent[initID] {
        b.mu.Unlock()
        return 
    }
    b.mu.Unlock()

	valueHash := sha256.Sum256(msg.Value)

	var doEcho, doReady, doDeliver bool
	var valueToBroadcast []byte

	b.mu.Lock()

	if msg.Type == Echo {
		if b.echoReceived[initID] == nil {
			b.echoReceived[initID] = make(map[ValueHash]map[core.NodeID]bool)
		}
		if b.echoReceived[initID][valueHash] == nil {
			b.echoReceived[initID][valueHash] = make(map[core.NodeID]bool)
		}
		b.echoReceived[initID][valueHash][msg.Sender] = true
	}

	if msg.Type == Ready {
		if b.readyReceived[initID] == nil {
			b.readyReceived[initID] = make(map[ValueHash]map[core.NodeID]bool)
		}
		if b.readyReceived[initID][valueHash] == nil {
			b.readyReceived[initID][valueHash] = make(map[core.NodeID]bool)
		}
		b.readyReceived[initID][valueHash][msg.Sender] = true
	}

	switch msg.Type {
	case Init: // when INIT(j,m) is received do
		// (2) if (first reception of INIT(j,-))
		if msg.Sender == initID && !b.echoSent[initID] {
			// then broadcast ECHO(j,m)
			b.echoSent[initID] = true
			valueToBroadcast = msg.Value
			doEcho = true
		}
	case Echo: // when ECHO(j,m) is received do
		// (3) if (ECHO(j,m) received from more than (n+t)/2 different processes) and (READY(j,m) not yet broadcast)
		if len(b.echoReceived[initID][valueHash]) > ((b.n+b.t)/2) && !b.readySent[initID] {
			// (4) then broadcast READY(j,m)
			b.readySent[initID] = true
			valueToBroadcast = msg.Value
			doReady = true
		}
	case Ready: // when READY(j,m) is received do
		// (6) if (READY(j,m) received from (t+1) different processes) and (READY(j,m) not yet broadcast)
		if len(b.readyReceived[initID][valueHash]) >= b.t+1 && !b.readySent[initID] {
			// (7) then broadcast READY(j,m)
			b.readySent[initID] = true
			valueToBroadcast = msg.Value
			doReady = true
		}
		// (9) if (READY(j,m) received from (2t+1) different processes) and (<j,m> not yet brb-delivered)
		if len(b.readyReceived[initID][valueHash]) >= 2*b.t+1 && !b.delivered[initID] {
			// (10) then BRB-deliver <j,m>
			b.delivered[initID] = true
			doDeliver = true
		}
	}

	b.mu.Unlock()

	if doEcho {
		b.broadcastMsg(Echo, initID, valueToBroadcast)
	}
	if doReady {
		b.broadcastMsg(Ready, initID, valueToBroadcast)
	}
	if doDeliver {
		b.manager.Deliver(initID, msg.Value)
	}
}

func (b *Bracha) broadcastMsg(msgType MsgType, initiator core.NodeID, value []byte) {
	msg := Message{Type: msgType, Sender: b.manager.ID(), Initiator: initiator, Value: value}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(msg); err != nil {
		fmt.Printf("[ERROR] Failed to encode message type %s: %v\n", msgType, err)
		return
	}
	b.manager.Broadcast(buf.Bytes())
}

func (b *Bracha) InitiateBroadcast(valueToSend []byte) {
	// (1) broadcast INIT(i, mi)
	b.broadcastMsg(Init, b.manager.ID(), valueToSend)
}
