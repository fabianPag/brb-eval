package codedmbrb

import (
	"brb-eval/internal/controller/api"
	"brb-eval/internal/node/algorithm/codedmbrb/ts"
	"brb-eval/internal/node/algorithm/codedmbrb/vc"
	"brb-eval/internal/node/core"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/kzg"
	"github.com/klauspost/reedsolomon"
)

type CodedMBRB struct {
	manager   core.Manager
	nodeIndex int
	senderID  int
	mu        sync.Mutex
	n         int
	t         int
	k         int
	parity    int
	d         int
	tau       int
	rsEncoder reedsolomon.Encoder

	pk kzg.ProvingKey
	vk kzg.VerifyingKey

	privateShare fr.Element
	masterPubKey bn254.G2Affine
	nodePubKeys  map[int]bn254.G2Affine

	storedShares    map[string]map[int][]byte
	storedFragments map[string]map[int][]byte
	hasSentForward  map[string]bool
	hasSentBundle   map[string]bool
	savedThresh     map[string][]byte
	hasDelivered    bool
	hasSigned       bool
	signedFor       string
}

type MBRBMessage struct {
	Type               string // "SEND", "FORWARD", "BUNDLE"
	Initiator          int
	Commitment         []byte           // kzg.Digest serialized C
	Fragment           *NetworkFragment // SEND/FORWARD fragment tuple (or nil for sentinel in FORWARD)
	BundleFragI        *NetworkFragment // BUNDLE: Sender tuple
	BundleFragJ        *NetworkFragment // BUNDLE: Receiver tuple (or nil for sentinel)
	SignatureShares    map[int][]byte   // SEND/FORWARD signature shares indexed by signer node ID.
	ThresholdSignature []byte           // BUNDLE: Aggregated threshold signature
}

type NetworkFragment struct {
	Data  []byte // Reed-Solomon shard bytes
	Proof []byte // Serialized kzg.OpeningProof (\pi_j)
	Index int    // The node index (j)
}

func NewCodedMBRB(m core.Manager, t int, d int, senderID int, publicCryptoParams api.PublicCryptoParams, privateCryptoParams api.PrivateCryptoParams) *CodedMBRB {
	// Reconstruct KZG keys
	var pk kzg.ProvingKey
	var vk kzg.VerifyingKey

	if _, err := pk.ReadFrom(bytes.NewReader(publicCryptoParams.PKBytes)); err != nil {
		panic("Failed to deserialize PK")
	}
	if _, err := vk.ReadFrom(bytes.NewReader(publicCryptoParams.VKBytes)); err != nil {
		panic("Failed to deserialize VK")
	}

	// Reconstruct Native BLS Threshold elements
	var privateShare fr.Element
	privateShare.SetBytes(privateCryptoParams.PrivateShareBytes)

	var masterPubKey bn254.G2Affine
	if err := masterPubKey.Unmarshal(publicCryptoParams.MasterPubKeyBytes); err != nil {
		panic("Failed to deserialize Master Public Key")
	}

	parsedNodePubKeys := make(map[int]bn254.G2Affine)
	for nodeID, pubKeyBytes := range publicCryptoParams.NodePubKeysBytes {
		var pk bn254.G2Affine
		if err := pk.Unmarshal(pubKeyBytes); err != nil {
			continue
		}
		parsedNodePubKeys[nodeID] = pk
	}

	n := m.NumPeers()
	if senderID < 0 || senderID >= n {
		panic(fmt.Sprintf("invalid designated sender: %d (n=%d)", senderID, n))
	}
	if publicCryptoParams.NumDataShards+publicCryptoParams.NumParityShards != n {
		panic("invalid shard config: data_shards + parity_shards must equal n")
	}

	tau := ((n + t) / 2) + 1
	k := publicCryptoParams.NumDataShards
	parity := publicCryptoParams.NumParityShards

	if n <= 3*t+2*d {
		panic(fmt.Sprintf("invalid parameters: require n > 3t + 2d, got n=%d t=%d d=%d", n, t, d))
	}

	if k > n-t-2*d {
		panic(fmt.Sprintf("invalid parameters: require k <= n - t - 2d, got k=%d n=%d t=%d d=%d", k, n, t, d))
	}

	rsEncoder, err := reedsolomon.New(k, parity)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize reed-solomon encoder: %v", err))
	}

	return &CodedMBRB{
		manager:         m,
		nodeIndex:       int(m.ID()),
		senderID:        senderID,
		n:               n,
		t:               t,
		k:               k,
		parity:          parity,
		d:               d,
		tau:             tau,
		rsEncoder:       rsEncoder,
		pk:              pk,
		vk:              vk,
		privateShare:    privateShare,
		masterPubKey:    masterPubKey,
		nodePubKeys:     parsedNodePubKeys,
		storedShares:    make(map[string]map[int][]byte),
		storedFragments: make(map[string]map[int][]byte),
		hasSentForward:  make(map[string]bool),
		hasSentBundle:   make(map[string]bool),
		savedThresh:     make(map[string][]byte),
	}
}

func (b *CodedMBRB) verifyShareByID(commitment []byte, signerID int, sig []byte) bool {
	pubKey, ok := b.nodePubKeys[signerID]
	if !ok {
		return false
	}
	return ts.VerifyShare(commitment, sig, pubKey)
}

func (b *CodedMBRB) cloneSignatures(src map[int][]byte) map[int][]byte {
	out := make(map[int][]byte, len(src))
	for id, sig := range src {
		out[id] = append([]byte(nil), sig...)
	}
	return out
}

func (b *CodedMBRB) validateShareSet(commitment []byte, shares map[int][]byte) bool {
	if len(shares) == 0 {
		return false
	}
	senderSig, ok := shares[b.senderID]
	if !ok || !b.verifyShareByID(commitment, b.senderID, senderSig) {
		return false
	}

	for signerID, sig := range shares {
		if !b.verifyShareByID(commitment, signerID, sig) {
			return false
		}
	}
	return true
}

func (b *CodedMBRB) signAndRecord(commitment []byte, cKey string) ([]byte, error) {
	if b.hasSigned && b.signedFor != cKey {
		return nil, fmt.Errorf("already signed a different commitment")
	}
	sig, err := ts.SignShare(commitment, b.privateShare)
	if err != nil {
		return nil, err
	}
	b.hasSigned = true
	b.signedFor = cKey
	if b.storedShares[cKey] == nil {
		b.storedShares[cKey] = make(map[int][]byte)
	}
	b.storedShares[cKey][b.nodeIndex] = sig
	return sig, nil
}

// Maps to Line 1: function computeFragVecCommit(m)
func (b *CodedMBRB) computeFragVecCommit(message []byte) (kzg.Digest, map[int]NetworkFragment, error) {
	shards, err := b.rsEncoder.Split(message)
	if err != nil {
		return kzg.Digest{}, nil, err
	}
	if err := b.rsEncoder.Encode(shards); err != nil {
		return kzg.Digest{}, nil, err
	}

	C, poly, err := vc.HashAndCommit(shards, b.pk)
	if err != nil {
		return kzg.Digest{}, nil, err
	}

	fragments := make(map[int]NetworkFragment)
	for i := 1; i <= b.n; i++ {
		proof, err := vc.Open(poly, i, b.pk)
		if err != nil {
			return kzg.Digest{}, nil, err
		}
		fragments[i] = NetworkFragment{
			Data:  append([]byte(nil), shards[i-1]...),
			Proof: serializeProof(proof),
			Index: i,
		}
	}

	return C, fragments, nil
}

func (b *CodedMBRB) verifyShardProof(C kzg.Digest, shard []byte, pi_j kzg.OpeningProof, j int) bool {
	claimedValue := vc.HashToFr(shard)
	return vc.Verify(C, pi_j, b.vk, claimedValue, j)
}

// Maps to Line 17: function MBRBbroadcast(m)
func (b *CodedMBRB) Broadcast(message []byte) error {
	if b.nodeIndex != b.senderID {
		return fmt.Errorf("only designated sender %d can broadcast", b.senderID)
	}

	// Frame the message with a 4-byte length prefix
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(message)))
	framedMessage := append(lengthBuf, message...)
	// Line 18: <C, {\tilde{m}_i}, {\pi_i}> = computeFragVecCommit(m)
	C, fragments, err := b.computeFragVecCommit(framedMessage)
	if err != nil {
		return err
	}

	cSerializer := C.Marshal()
	// Line 19
	senderSig, err := ts.SignShare(cSerializer, b.privateShare)
	if err != nil {
		return err
	}
	senderShares := map[int][]byte{b.senderID: senderSig}

	safeSenderShares := b.cloneSignatures(senderShares)

	// Line 20: for all j in N, send <send, C, \tilde{m}_j, j, \pi_j> to node j
	for j := 1; j <= b.n; j++ {
		nf := fragments[j]

		sendMsg := MBRBMessage{
			Type:            "SEND",
			Initiator:       b.nodeIndex,
			Commitment:      cSerializer,
			Fragment:        &nf,
			SignatureShares: safeSenderShares,
		}
		msgBytes, _ := json.Marshal(sendMsg)

		// Send to node (Nodes in config are 0-indexed, j is 1-indexed)
		go b.manager.Send(core.NodeID(j-1), msgBytes)
	}

	return nil
}

func (b *CodedMBRB) HandleIncoming(fromID core.NodeID, payload []byte) {
	var msg MBRBMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "SEND":
		b.uponSendArrival(fromID, &msg)
	case "FORWARD":
		b.uponForwardArrival(&msg)
	case "BUNDLE":
		b.uponBundleArrival(&msg)
	}

	b.evaluateDelivery()
}

// Maps to Line 21: upon receiving <send, C, v, j, \pi_j> from the designated sender
func (b *CodedMBRB) uponSendArrival(from core.NodeID, msg *MBRBMessage) {
	var C kzg.Digest
	if err := C.Unmarshal(msg.Commitment); err != nil || msg.Fragment == nil {
		return
	}

	cKey := string(msg.Commitment)

	if int(from) != b.senderID {
		return
	}

	if !b.validateShareSet(msg.Commitment, msg.SignatureShares) {
		return
	}

	b.mu.Lock()
	if b.hasSentForward[cKey] || (b.hasSigned && b.signedFor != cKey) {
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	var pi_j kzg.OpeningProof
	if err := deserializeProof(&pi_j, msg.Fragment.Proof); err != nil {
		return
	}
	// Line 22: if has_sent_forward = false and isValid(C, v, pi_j)
	if !b.verifyShardProof(C, msg.Fragment.Data, pi_j, msg.Fragment.Index) {
		return
	}

	b.mu.Lock()
	if b.hasSentForward[cKey] || (b.hasSigned && b.signedFor != cKey) {
		b.mu.Unlock()
		return
	}

	if b.storedShares[cKey] == nil {
		b.storedShares[cKey] = make(map[int][]byte)
	}
	for signerID, sig := range msg.SignatureShares {
		b.storedShares[cKey][signerID] = append([]byte(nil), sig...)
	}

	sigma_i, err := b.signAndRecord(msg.Commitment, cKey)
	if err != nil {
		b.mu.Unlock()
		return
	}

	if b.storedFragments[cKey] == nil {
		b.storedFragments[cKey] = make(map[int][]byte)
	}
	b.storedFragments[cKey][msg.Fragment.Index] = append([]byte(nil), msg.Fragment.Data...)

	b.hasSentForward[cKey] = true
	sharesForForward := b.cloneSignatures(msg.SignatureShares)
	sharesForForward[b.nodeIndex] = append([]byte(nil), sigma_i...)
	b.mu.Unlock()

	// Line 25: forall k in N, send <forward, C, \sigma_i, v, j, \pi_j>
	forwardMsg := MBRBMessage{
		Type:            "FORWARD",
		Initiator:       msg.Initiator,
		Commitment:      msg.Commitment,
		Fragment:        msg.Fragment,
		SignatureShares: sharesForForward,
	}
	msgBytes, _ := json.Marshal(forwardMsg)

	b.manager.Broadcast(msgBytes)
}

// Maps to Line 26: upon receiving <forward, C, \sigma_l, v, j, \pi_j> from node l
func (b *CodedMBRB) uponForwardArrival(msg *MBRBMessage) {
	var C kzg.Digest
	if err := C.Unmarshal(msg.Commitment); err != nil || msg.Fragment == nil {
		return
	}

	cKey := string(msg.Commitment)

	if !b.validateShareSet(msg.Commitment, msg.SignatureShares) {
		return
	}

	var pi_j kzg.OpeningProof
	if err := deserializeProof(&pi_j, msg.Fragment.Proof); err != nil {
		return
	}
	if !b.verifyShardProof(C, msg.Fragment.Data, pi_j, msg.Fragment.Index) {
		return
	}

	b.mu.Lock()
	if b.hasSigned && b.signedFor != cKey {
		b.mu.Unlock()
		return
	}

	if b.storedShares[cKey] == nil {
		b.storedShares[cKey] = make(map[int][]byte)
	}
	for signerID, sig := range msg.SignatureShares {
		b.storedShares[cKey][signerID] = append([]byte(nil), sig...)
	}

	if b.storedFragments[cKey] == nil {
		b.storedFragments[cKey] = make(map[int][]byte)
	}
	b.storedFragments[cKey][msg.Fragment.Index] = append([]byte(nil), msg.Fragment.Data...)

	shouldSendSentinel := false
	var sigma_i []byte
	if !b.hasSentForward[cKey] {
		sig, err := b.signAndRecord(msg.Commitment, cKey)
		if err == nil {
			sigma_i = sig
			shouldSendSentinel = true
		}
		b.hasSentForward[cKey] = true
	}

	b.mu.Unlock()

	var sharesForForward map[int][]byte
	if shouldSendSentinel {
		sharesForForward = make(map[int][]byte)
		if senderSig, ok := msg.SignatureShares[b.senderID]; ok {
			sharesForForward[b.senderID] = append([]byte(nil), senderSig...)
		}
		sharesForForward[b.nodeIndex] = append([]byte(nil), sigma_i...)
	}

	// Line 31-34: if has_sent_forward = false, sign and forward sentinel value
	if shouldSendSentinel {
		forwardMsg := MBRBMessage{
			Type:            "FORWARD",
			Initiator:       msg.Initiator,
			Commitment:      msg.Commitment,
			Fragment:        nil, // Sending sentinel value
			SignatureShares: sharesForForward,
		}
		msgBytes, _ := json.Marshal(forwardMsg)
		b.manager.Broadcast(msgBytes)
	}
}

// Maps to Line 35: wait until \exists C' st get_thresh_sig(C') != nil && len(v) >= k && not evaluated
func (b *CodedMBRB) evaluateDelivery() {
	b.mu.Lock()
	if b.hasDelivered {
		b.mu.Unlock()
		return
	}

	var targetCKey string
	var targetShares map[int][]byte
	var targetFragments map[int][]byte
	var targetThreshSig []byte
	var hasThresh bool

	for cKey, shares := range b.storedShares {
		fragments := b.storedFragments[cKey]
		if len(fragments) < b.k {
			continue
		}

		existingThresh, exists := b.savedThresh[cKey]
		if !exists && len(shares) < b.tau {
			continue
		}

		targetCKey = cKey
		targetShares = b.cloneSignatures(shares)
		targetFragments = make(map[int][]byte)
		for k, v := range fragments {
			targetFragments[k] = append([]byte(nil), v...)
		}
		targetThreshSig = existingThresh
		hasThresh = exists
		break
	}
	b.mu.Unlock()

	if targetCKey == "" {
		return
	}

	CBytes := []byte(targetCKey)

	// Get Thresh Sig
	if !hasThresh {
		agg, err := ts.Combine(targetShares, b.tau)
		if err != nil || !ts.Verify(CBytes, agg, b.masterPubKey) {
			return
		}
		targetThreshSig = agg
		b.mu.Lock()
		b.savedThresh[targetCKey] = agg
		b.mu.Unlock()
	}

	// Line 36: m <- ECC^{-1}(\tilde{m}_{j \in E_C})
	reconstructedShards, err := b.reconstructAllShards(targetFragments)
	if err != nil {
		return
	}

	// Line 37: C'' <- vec_commit_hash(m)
	recreatedCommitment, poly, err := vc.HashAndCommit(reconstructedShards, b.pk)
	if err != nil {
		return
	}

	// Line 38: If the reconstructed digest doesn't match the commitment, return
	if string(recreatedCommitment.Marshal()) != targetCKey {
		return
	}

	decodedMsg, err := b.decodeFramedMessage(reconstructedShards)
	if err != nil {
		return
	}

	b.mu.Lock()
	sendBundle := false
	if !b.hasSentBundle[targetCKey] {
		b.hasSentBundle[targetCKey] = true
		sendBundle = true
	}
	doDeliver := !b.hasDelivered
	if doDeliver {
		b.hasDelivered = true
	}
	b.mu.Unlock()

	// Line 40: forall j in N, send <bundle, C, frag_i, frag_j, \Sigma_C> to j
	if sendBundle {
		iIndex := b.nodeIndex + 1
		proofI, err := vc.Open(poly, iIndex, b.pk)
		if err == nil {
			fragI := NetworkFragment{
				Data:  append([]byte(nil), reconstructedShards[iIndex-1]...),
				Proof: serializeProof(proofI),
				Index: iIndex,
			}

			for j := 1; j <= b.n; j++ {
				proofJ, err := vc.Open(poly, j, b.pk)
				if err != nil {
					continue
				}
				fragJ := NetworkFragment{
					Data:  append([]byte(nil), reconstructedShards[j-1]...),
					Proof: serializeProof(proofJ),
					Index: j,
				}
				bundleMsg := MBRBMessage{
					Type:               "BUNDLE",
					Initiator:          b.senderID,
					Commitment:         CBytes,
					BundleFragI:        &fragI,
					BundleFragJ:        &fragJ,
					ThresholdSignature: targetThreshSig,
				}
				msgBytes, _ := json.Marshal(bundleMsg)
				go b.manager.Send(core.NodeID(j-1), msgBytes)
			}
		}
	}

	if doDeliver {
		// Line 41: trigger MBRBdeliver(m)
		b.manager.Deliver(core.NodeID(b.senderID), decodedMsg)
	}
}

// Maps to Line 42: upon receiving <bundle, C, E_C, \sigma>
func (b *CodedMBRB) uponBundleArrival(msg *MBRBMessage) {
	b.mu.Lock()
	hasDelivered := b.hasDelivered
	b.mu.Unlock()

	if hasDelivered {
		return
	}

	cKey := string(msg.Commitment)

	// Line 43: Validate threshold signature
	if !ts.Verify(msg.Commitment, msg.ThresholdSignature, b.masterPubKey) {
		return
	}

	var C kzg.Digest
	if err := C.Unmarshal(msg.Commitment); err != nil {
		return
	}

	validI := false
	if msg.BundleFragI != nil {
		var pi kzg.OpeningProof
		if err := deserializeProof(&pi, msg.BundleFragI.Proof); err == nil {
			validI = b.verifyShardProof(C, msg.BundleFragI.Data, pi, msg.BundleFragI.Index)
		}
	}

	validJ := false
	if msg.BundleFragJ != nil {
		var pi kzg.OpeningProof
		if err := deserializeProof(&pi, msg.BundleFragJ.Proof); err == nil {
			validJ = b.verifyShardProof(C, msg.BundleFragJ.Data, pi, msg.BundleFragJ.Index)
		}
	} else {
		validJ = true // Sentinel value (nil) is valid for this fragment
	}

	if !validI || !validJ {
		return
	}

	b.mu.Lock()
	if b.savedThresh[cKey] == nil {
		b.savedThresh[cKey] = append([]byte(nil), msg.ThresholdSignature...)
	}

	if msg.BundleFragI != nil {
		if b.storedFragments[cKey] == nil {
			b.storedFragments[cKey] = make(map[int][]byte)
		}
		b.storedFragments[cKey][msg.BundleFragI.Index] = append([]byte(nil), msg.BundleFragI.Data...)
	}

	if msg.BundleFragJ != nil {
		if b.storedFragments[cKey] == nil {
			b.storedFragments[cKey] = make(map[int][]byte)
		}
		b.storedFragments[cKey][msg.BundleFragJ.Index] = append([]byte(nil), msg.BundleFragJ.Data...)
	}

	shouldForward := !b.hasSentBundle[cKey] && msg.BundleFragJ != nil && msg.BundleFragJ.Index == b.nodeIndex+1
	if shouldForward {
		b.hasSentBundle[cKey] = true
	}
	b.mu.Unlock()

	if shouldForward {
		forwardBundle := MBRBMessage{
			Type:               "BUNDLE",
			Initiator:          msg.Initiator,
			Commitment:         msg.Commitment,
			BundleFragI:        msg.BundleFragJ,
			BundleFragJ:        nil,
			ThresholdSignature: msg.ThresholdSignature,
		}
		msgBytes, _ := json.Marshal(forwardBundle)
		b.manager.Broadcast(msgBytes)
	}
}

func (b *CodedMBRB) reconstructAllShards(fragments map[int][]byte) ([][]byte, error) {
	shards := make([][]byte, b.n)

	var shardSize int
	for _, fragment := range fragments {
		if len(fragment) > 0 {
			shardSize = len(fragment)
			break
		}
	}

	for index, fragment := range fragments {
		if index < 1 || index > b.n {
			continue
		}
		shards[index-1] = make([]byte, shardSize)
		copy(shards[index-1], fragment)
	}

	if err := b.rsEncoder.Reconstruct(shards); err != nil {
		return nil, err
	}

	return shards, nil
}

func (b *CodedMBRB) decodeFramedMessage(shards [][]byte) ([]byte, error) {
	var reconstructed bytes.Buffer
	maxDataLen := b.k * len(shards[0])
	if err := b.rsEncoder.Join(&reconstructed, shards, maxDataLen); err != nil {
		return nil, err
	}

	raw := reconstructed.Bytes()
	if len(raw) < 4 {
		return nil, fmt.Errorf("reconstructed payload too short: %d", len(raw))
	}

	msgLen := int(binary.BigEndian.Uint32(raw[:4]))
	totalLen := 4 + msgLen
	if totalLen > len(raw) {
		return nil, fmt.Errorf("invalid framed length: %d > %d", totalLen, len(raw))
	}

	return append([]byte(nil), raw[4:totalLen]...), nil
}

func serializeProof(p kzg.OpeningProof) []byte {
	var b bytes.Buffer
	p.WriteTo(&b)
	return b.Bytes()
}

func deserializeProof(p *kzg.OpeningProof, data []byte) error {
	_, err := p.ReadFrom(bytes.NewReader(data))
	return err
}
