/*
	Implementation of the AFRT MBRB algorithm from the paper "Asynchronous Byzantine reliable broadcast with a message adversary"
	DOI: 10.1016/j.tcs.2023.114110
	https://www.sciencedirect.com/science/article/abs/pii/S0304397523004231
*/

package afrt

import (
	"brb-eval/internal/node/core"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"sync"
)

type Bundle struct {
	Initiator core.NodeID
	SeqNum    int
	Value     []byte
	Sigs      map[core.NodeID][]byte
}

type MsgKey struct {
	Initiator core.NodeID
	SeqNum    int
	ValueHash [32]byte
}

type InstanceKey struct {
	Initiator core.NodeID
	SeqNum    int
}

type AFRT struct {
	manager   core.Manager
	mu        sync.Mutex
	n         int
	t         int
	d         int
	privKey   ed25519.PrivateKey
	pubKeys   map[core.NodeID]ed25519.PublicKey
	savedSigs map[MsgKey]map[core.NodeID][]byte
	delivered map[InstanceKey]bool
	hasSigned map[InstanceKey]bool
}

func NewAFRT(m core.Manager, t int, d int, priv ed25519.PrivateKey, pubs map[core.NodeID]ed25519.PublicKey) *AFRT {
	n := m.NumPeers()

	if n <= 3*t+2*d {
		panic(fmt.Sprintf("invalid parameters: require n > 3t + 2d, got n=%d t=%d d=%d", n, t, d))
	}

	return &AFRT{
		manager:   m,
		n:         n,
		t:         t,
		d:         d,
		privKey:   priv,
		pubKeys:   pubs,
		savedSigs: make(map[MsgKey]map[core.NodeID][]byte),
		delivered: make(map[InstanceKey]bool),
		hasSigned: make(map[InstanceKey]bool),
	}
}

func (b *AFRT) HandleIncoming(fromID core.NodeID, payload []byte) {
	var bundle Bundle
	buf := bytes.NewReader(payload)
	if err := gob.NewDecoder(buf).Decode(&bundle); err != nil {
		fmt.Printf("[ERROR] Failed to decode bundle message from node %d: %v\n", fromID, err)
		return
	}

	if bundle.SeqNum != 0 {
		return
	}
	valueHash := sha256.Sum256(bundle.Value)

	msgKey := MsgKey{Initiator: bundle.Initiator, SeqNum: bundle.SeqNum, ValueHash: valueHash}
	instKey := InstanceKey{Initiator: bundle.Initiator, SeqNum: bundle.SeqNum}

	// (3) if ((−, sn, j) not already mbrb-delivered
	b.mu.Lock()
	if b.delivered[instKey] {
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	signedMsg, ok := bundle.Sigs[bundle.Initiator]
	if !ok || !b.verify(valueHash, bundle.SeqNum, bundle.Initiator, bundle.Initiator, signedMsg) {
		return
	}

	validSigs := make(map[core.NodeID][]byte)
	for signer, sig := range bundle.Sigs {
		if b.verify(valueHash, bundle.SeqNum, bundle.Initiator, signer, sig) {
			validSigs[signer] = sig
		}
	}

	b.mu.Lock()

	if b.delivered[instKey] {
		b.mu.Unlock()
		return
	}

	if b.savedSigs[msgKey] == nil {
		b.savedSigs[msgKey] = make(map[core.NodeID][]byte)
	}

	// (4) Save all unsaved valid signatures for (m,sn,j) of sigs;
	for signer, sig := range validSigs {
		b.savedSigs[msgKey][signer] = append([]byte(nil), sig...)
	}

	doSign := false
	var mySig []byte

	// (5) if ((-,sn,j) not already signed by pi) then
	if !b.hasSigned[instKey] {
		b.hasSigned[instKey] = true
		doSign = true
	}

	doDeliver := false
	// (9) if strictly more than (n+t)/2 signatures for (m,sn,j) are saved
	if len(b.savedSigs[msgKey]) > (b.n+b.t)/2 {
		b.delivered[instKey] = true
		doDeliver = true
	}

	sigsToSend := make(map[core.NodeID][]byte)
	for k, v := range b.savedSigs[msgKey] {
		sigsToSend[k] = append([]byte(nil), v...)
	}
	b.mu.Unlock()

	// (6) save signature for (m,sn,j) by pi
	if doSign {
		mySig = b.sign(valueHash, bundle.SeqNum, bundle.Initiator)

		b.mu.Lock()
		b.savedSigs[msgKey][b.manager.ID()] = append([]byte(nil), mySig...)
		sigsToSend[b.manager.ID()] = append([]byte(nil), mySig...)
		b.mu.Unlock()

		// (7) broadcast bundle(m, sn, j, {all saved signatures for (m, sn, j)})
		b.sendBundle(msgKey, bundle.Value, sigsToSend)
	}

	if doDeliver {
		// (10) broadcast bundle(m, sn, j, {all saved signatures for (m, sn, j)});
		if !doSign {
			b.sendBundle(msgKey, bundle.Value, sigsToSend)
		}
		// (11) mbrb_deliver(m, sn, j)
		b.manager.Deliver(bundle.Initiator, []byte(bundle.Value))
	}
}

func (b *AFRT) InitiateBroadcast(value []byte) {
	const sn = 0 // hardcoded for single-shot implementation
	valueHash := sha256.Sum256(value)
	msgKey := MsgKey{Initiator: b.manager.ID(), SeqNum: sn, ValueHash: valueHash}
	instKey := InstanceKey{Initiator: b.manager.ID(), SeqNum: sn}

	sig := b.sign(valueHash, sn, b.manager.ID())

	b.mu.Lock()
	if b.savedSigs[msgKey] == nil {
		b.savedSigs[msgKey] = make(map[core.NodeID][]byte)
	}
	b.savedSigs[msgKey][b.manager.ID()] = append([]byte(nil), sig...)
	b.hasSigned[instKey] = true

	sigsToSend := make(map[core.NodeID][]byte)
	for k, v := range b.savedSigs[msgKey] {
		sigsToSend[k] = append([]byte(nil), v...)
	}
	b.mu.Unlock()

	b.sendBundle(msgKey, value, sigsToSend)
}

func (b *AFRT) sign(valueHash [32]byte, sn int, origin core.NodeID) []byte {
	msg := fmt.Sprintf("%x|%d|%d", valueHash, sn, origin)
	return ed25519.Sign(b.privKey, []byte(msg))
}

func (b *AFRT) verify(valueHash [32]byte, sn int, origin core.NodeID, signer core.NodeID, sig []byte) bool {
	pub, ok := b.pubKeys[signer]
	if !ok {
		return false
	}
	msg := fmt.Sprintf("%x|%d|%d", valueHash, sn, origin)
	return ed25519.Verify(pub, []byte(msg), sig)
}

func (b *AFRT) sendBundle(key MsgKey, value []byte, sigs map[core.NodeID][]byte) {
	bundle := Bundle{
		Initiator: key.Initiator,
		SeqNum:    key.SeqNum,
		Value:     value,
		Sigs:      sigs,
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(bundle); err != nil {
		fmt.Printf("[ERROR] Failed to encode bundle message: %v\n", err)
		return
	}
	data := buf.Bytes()

	b.manager.Broadcast(data)
}
