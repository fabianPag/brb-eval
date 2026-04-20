/*
	Implementation of Raynal's version of Bracha's algorithm from the book "Fault-Tolerant Message-Passing Distributed Systems"
	DOI: 10.1007/978-3-319-94141-7
	https://link.springer.com/book/10.1007/978-3-319-94141-7
*/

package bracha

import (
	"brb-eval/internal/node/core"
	"bytes"
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

type BroadcastID struct {
	Initiator core.NodeID
	Value     string
}

type Bracha struct {
	mu            sync.Mutex
	manager       core.Manager
	echoReceived  map[BroadcastID]map[core.NodeID]bool
	echoSent      map[BroadcastID]bool
	readyReceived map[BroadcastID]map[core.NodeID]bool
	readySent     map[BroadcastID]bool
	delivered     map[BroadcastID]bool
	n             int
	t             int
}

func NewBracha(m core.Manager) *Bracha {
	nCount := m.NumPeers()

	return &Bracha{
		manager:       m,
		n:             nCount,
		t:             (nCount - 1) / 3,
		echoReceived:  make(map[BroadcastID]map[core.NodeID]bool),
		echoSent:      make(map[BroadcastID]bool),
		readyReceived: make(map[BroadcastID]map[core.NodeID]bool),
		readySent:     make(map[BroadcastID]bool),
		delivered:     make(map[BroadcastID]bool),
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

	key := BroadcastID{Initiator: msg.Initiator, Value: string(msg.Value)}

	var doEcho, doReady, doDeliver bool

	b.mu.Lock()
	if msg.Type == Echo {
		if b.echoReceived[key] == nil {
			b.echoReceived[key] = make(map[core.NodeID]bool)
		}
		b.echoReceived[key][msg.Sender] = true
	}

	if msg.Type == Ready {
		if b.readyReceived[key] == nil {
			b.readyReceived[key] = make(map[core.NodeID]bool)
		}
		b.readyReceived[key][msg.Sender] = true
	}

	switch msg.Type {
	case Init: // when INIT(j,m) is recieved do:
		if !b.echoSent[key] { // if (first reception of INIT(j,-) and ECHO(j,m) not yet broadcast)
			b.echoSent[key] = true
			//b.broadcastMsg(Echo, msg.Initiator, msg.Value)
			doEcho = true
		}
	case Echo: // when ECHO(j,m) is recieved do
		// if (ECHO(j,m) receiveed from more than (n+t)/2 different processes)
		if len(b.echoReceived[key]) > ((b.n + b.t) / 2) {
			if !b.echoSent[key] { // if (ECHO(j,m) not yet broadcast) then broadcast ECHO(j,m)
				b.echoSent[key] = true
				//b.broadcastMsg(Echo, msg.Initiator, msg.Value)
				doEcho = true
			}
			if !b.readySent[key] { // if (READY(j,m) not yet broadcast) then broadcast ECHO(j,m)
				b.readySent[key] = true
				//b.broadcastMsg(Ready, msg.Initiator, msg.Value)
				doReady = true
			}
		}
	case Ready: // when READY(j,m) is received do
		if len(b.readyReceived[key]) >= b.t+1 {
			if !b.echoSent[key] { // if (ECHO(j,m) not yet broadcast) then broadcast ECHO(j,m)
				b.echoSent[key] = true
				//b.broadcastMsg(Echo, msg.Initiator, msg.Value)
				doEcho = true
			}
			if !b.readySent[key] { // if (READY(j,m) not yet broadcast) then broadcast ECHO(j,m)
				b.readySent[key] = true
				//b.broadcastMsg(Ready, msg.Initiator, msg.Value)
				doReady = true
			}
		}
		if len(b.readyReceived[key]) >= 2*b.t+1 && !b.delivered[key] {
			b.delivered[key] = true
			//b.manager.Deliver(msg.Initiator, []byte(msg.Value))
			doDeliver = true
		}
	}

	b.mu.Unlock()

	if doEcho {
		b.broadcastMsg(Echo, msg.Initiator, msg.Value)
	}
	if doReady {
		b.broadcastMsg(Ready, msg.Initiator, msg.Value)
	}
	if doDeliver {
		b.manager.Deliver(msg.Initiator, []byte(msg.Value))
	}
}

func (b *Bracha) broadcastMsg(msgType MsgType, initiator core.NodeID, value []byte) {
	msg := Message{Type: msgType, Sender: b.manager.ID(), Initiator: initiator, Value: value}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(msg); err != nil {
		fmt.Printf("[ERROR] Failed to encode message type %s: %v\n", msgType, err)
		return
	}
	data := buf.Bytes()
	b.manager.Broadcast(data)
}

func (b *Bracha) InitiateBroadcast(valueToSend []byte) {
	b.broadcastMsg(Init, b.manager.ID(), valueToSend)
}
