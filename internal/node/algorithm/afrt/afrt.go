/*
	Implementation of the AFRT MBRB algorithm from the paper "Asynchronous Byzantine reliable broadcast with a message adversary"
	DOI: 10.1016/j.tcs.2023.114110
	https://www.sciencedirect.com/science/article/abs/pii/S0304397523004231
*/

package afrt

import (
	"brb-eval/internal/node/core"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"sync"
)

type Bundle struct {
	Initiator core.NodeID            `json:"initiator"`
	SeqNum    int                    `json:"sn"`
	Value     string                 `json:"val"`
	Sigs      map[core.NodeID][]byte `json:"sigs"`
}

type MsgKey struct {
	Initiator core.NodeID
	SeqNum    int
}

type AFRT struct {
	manager   core.Manager
	mu        sync.Mutex
	n         int
	t         int
	privKey   ed25519.PrivateKey
	pubKeys   map[core.NodeID]ed25519.PublicKey
	savedSigs map[MsgKey]map[core.NodeID][]byte
	delivered map[MsgKey]bool
	hasSigned map[MsgKey]bool
}

func NewAFRT(m core.Manager, t int, priv ed25519.PrivateKey, pubs map[core.NodeID]ed25519.PublicKey) *AFRT {
	return &AFRT{
		manager:   m,
		n:         m.NumPeers(),
		t:         t,
		privKey:   priv,
		pubKeys:   pubs,
		savedSigs: make(map[MsgKey]map[core.NodeID][]byte),
		delivered: make(map[MsgKey]bool),
		hasSigned: make(map[MsgKey]bool),
	}
}

func (b *AFRT) HandleIncoming(fromID core.NodeID, msg []byte) {
	var bundle Bundle
	if err := json.Unmarshal(msg, &bundle); err != nil {
		fmt.Printf("[ERROR] Failed to unmarshal message from node %d: %v\n", fromID, err)
		return
	}

	if bundle.SeqNum != 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	key := MsgKey{Initiator: bundle.Initiator, SeqNum: bundle.SeqNum}

	// (3) if ((−, sn, j) not already mbrb-delivered
	if b.delivered[key] {
		return
	}

	signedMsg, ok := bundle.Sigs[bundle.Initiator]
	if !ok || !b.verify(bundle.Value, bundle.SeqNum, bundle.Initiator, bundle.Initiator, signedMsg) {
		return
	}

	if b.savedSigs[key] == nil {
		b.savedSigs[key] = make(map[core.NodeID][]byte)
	}

	// (4) Save all unsaved valid signatures for (m,sn,j) of sigs;
	for signer, signedMsg := range bundle.Sigs {
		if _, exists := b.savedSigs[key][signer]; exists {
			continue
		}

		if b.verify(bundle.Value, bundle.SeqNum, bundle.Initiator, signer, signedMsg) {
			fmt.Printf("Saved valid signature from %d of message with value %s from initiator %d \n", signer, bundle.Value, bundle.Initiator)
			b.savedSigs[key][signer] = signedMsg
		}
	}
	// (5) if ((-,sn,j) not already signed by pi) then
	if !b.hasSigned[key] {
		// (6) save signature for (m,sn,j) by pi
		fmt.Printf("Signed message with value %s from initiator %d \n", bundle.Value, bundle.Initiator)
		b.savedSigs[key][b.manager.ID()] = b.sign(bundle.Value, bundle.SeqNum, bundle.Initiator)
		b.hasSigned[key] = true
		//(7) broadcast bundle(m, sn, j, {all saved signatures for (m, sn, j)})
		b.sendBundle(key, bundle.Value)
	}

	// (9) if strictly more than (n+t)/2 signatures for (m,sn,j) are saved
	if len(b.savedSigs[key]) > (b.n+b.t)/2 {
		// (10) broadcast bundle(m, sn, j, {all saved signatures for (m, sn, j)});
		b.sendBundle(key, bundle.Value)
		// (11) mbrb_deliver(m, sn, j)
		b.delivered[key] = true
		b.manager.Deliver(bundle.Initiator, []byte(bundle.Value))
	}
}

func (b *AFRT) Broadcast(value string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	const sn = 0 // hardcoded for single-shot implementation

	key := MsgKey{Initiator: b.manager.ID(), SeqNum: sn}
	sig := b.sign(value, sn, b.manager.ID())

	if b.savedSigs[key] == nil {
		b.savedSigs[key] = make(map[core.NodeID][]byte)
	}
	b.savedSigs[key][b.manager.ID()] = sig
	b.hasSigned[key] = true
	b.sendBundle(key, value)
}

func (b *AFRT) sign(value string, sn int, origin core.NodeID) []byte {
	msg := fmt.Sprintf("%s|%d|%d", value, sn, origin)
	return ed25519.Sign(b.privKey, []byte(msg))
}

func (b *AFRT) verify(value string, sn int, origin core.NodeID, signer core.NodeID, sig []byte) bool {
	pub, ok := b.pubKeys[signer]
	if !ok {
		return false
	}
	msg := fmt.Sprintf("%s|%d|%d", value, sn, origin)
	return ed25519.Verify(pub, []byte(msg), sig)
}

func (b *AFRT) sendBundle(key MsgKey, value string) {
	bundle := Bundle{
		Initiator: key.Initiator,
		SeqNum:    key.SeqNum,
		Value:     value,
		Sigs:      b.savedSigs[key],
	}
	data, _ := json.Marshal(bundle)
	b.manager.Broadcast(data)
}
