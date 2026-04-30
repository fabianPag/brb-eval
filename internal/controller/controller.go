package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"brb-eval/internal/controller/api"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/kzg"
)

type Controller struct {
	mu                  sync.Mutex
	targetNodes         int
	designatedSender    int
	t                   int
	d                   int
	numSilent           int
	numIsolated         int
	algorithm           string
	payloadSize         int
	requiredDeliveries  int
	nodeMetrics         []api.MetricsData
	registeredNodes     map[int]api.PeerInfo
	readyNodes          map[int]bool
	metricsReceived     map[int]bool
	doneNodes           map[int]bool
	allRegistered       chan struct{}
	allReady            chan struct{}
	publicCryptoParams  api.PublicCryptoParams
	privateCryptoParams map[int]api.PrivateCryptoParams
	allDone             chan struct{}
	allMetricsReceived  chan struct{}
	twinsSender         bool
}

type metricsFile struct {
	Algorithm        string            `json:"algorithm"`
	PayloadSize      int               `json:"payload_size"`
	N                int               `json:"n"`
	T                int               `json:"t"`
	D                int               `json:"d"`
	DesignatedSender int               `json:"designated_sender"`
	NumSilent        int               `json:"num_silent"`
	NumIsolated      int               `json:"num_isolated"`
	NodeMetrics      []api.MetricsData `json:"node_metrics"`
}

func New(targetNodes int, t int, d int, algorithm string, payloadSize int, twinsSender bool, numSilent int, numIsolated int) *Controller {
	return &Controller{
		targetNodes:        targetNodes,
		designatedSender:   0,
		t:                  t,
		d:                  d,
		numSilent:          numSilent,
		numIsolated:        numIsolated,
		algorithm:          algorithm,
		payloadSize:        payloadSize,
		nodeMetrics:        make([]api.MetricsData, targetNodes),
		registeredNodes:    make(map[int]api.PeerInfo),
		readyNodes:         make(map[int]bool),
		doneNodes:          make(map[int]bool),
		metricsReceived:    make(map[int]bool),
		allRegistered:      make(chan struct{}),
		allReady:           make(chan struct{}),
		allDone:            make(chan struct{}),
		allMetricsReceived: make(chan struct{}),
		twinsSender:        twinsSender,
	}
}

func (c *Controller) Run(addr string, shouldSaveMetrics bool) error {
	c.InitializeCrypto(c.targetNodes)

	http.HandleFunc("/register", c.handleRegister)
	http.HandleFunc("/ready", c.handleReady)
	http.HandleFunc("/done", c.handleDone)
	http.HandleFunc("/metrics", c.handleMetrics)
	fmt.Println("Starting controller at", addr)

	go http.ListenAndServe(addr, nil)
	<-c.allMetricsReceived

	if shouldSaveMetrics {
		if err := c.saveMetricsToFile("metrics.json"); err != nil {
			fmt.Printf("[ERROR] Failed to save metrics: %v\n", err)
		}
	}

	return nil
}

func (c *Controller) handleDone(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	c.mu.Lock()

	if !c.doneNodes[id] {
		c.doneNodes[id] = true
		fmt.Printf("Node %d is done\n", id)

		if len(c.doneNodes) == c.targetNodes {
			fmt.Println("All nodes done. Waiting for 3 seconds to let any in-flight algorithm messages arrive.")
			go func() {
				time.Sleep(3 * time.Second)
				fmt.Println("Unblocking all nodes.")
				close(c.allDone)
			}()
		}
	}

	c.mu.Unlock()

	// Block until all nodes have delivered
	<-c.allDone

	w.WriteHeader(http.StatusOK)
}

func (c *Controller) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var metrics api.MetricsData
	if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if metrics.NodeID < 0 || metrics.NodeID >= c.targetNodes {
		http.Error(w, "Invalid Node ID", http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	c.nodeMetrics[metrics.NodeID] = metrics
	c.metricsReceived[metrics.NodeID] = true
	allReceived := len(c.metricsReceived) == c.targetNodes
	c.mu.Unlock()

	fmt.Printf("Node %d sent metrics\n", metrics.NodeID)

	w.WriteHeader(http.StatusOK)

	if allReceived {
		close(c.allMetricsReceived)
	}
}

func (c *Controller) saveMetricsToFile(filename string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	output := metricsFile{
		Algorithm:        c.algorithm,
		PayloadSize:      c.payloadSize,
		N:                c.targetNodes,
		T:                c.t,
		D:                c.d,
		DesignatedSender: c.designatedSender,
		NumSilent:        c.numSilent,
		NumIsolated:      c.numIsolated,
		NodeMetrics:      c.nodeMetrics,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	err = os.WriteFile(filename, data, 0644)
	if err == nil {
		fmt.Printf("Metrics saved to %s\n", filename)
	}
	return err
}

func (c *Controller) handleRegister(w http.ResponseWriter, r *http.Request) {
	var node api.PeerInfo
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	fmt.Printf("Node %d registered from %s\n", node.ID, node.Address)

	c.mu.Lock()

	c.registeredNodes[node.ID] = node
	allRegistered := len(c.registeredNodes) == c.targetNodes
	if allRegistered {
		fmt.Println("All nodes registered. Sending configs.")
		close(c.allRegistered)
	}
	c.mu.Unlock()

	// Block until all nodes registered
	if !allRegistered {
		<-c.allRegistered
	}

	role := "honest"
	if node.ID == c.designatedSender {
		if c.twinsSender {
			role = "twins_sender"
		} else {
			role = "honest"
		}
	} else {
		if int(node.ID) >= c.targetNodes-c.numSilent {
			role = "silent"
		} else if int(node.ID) >= c.targetNodes-c.numSilent-c.numIsolated {
			role = "isolated"
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	config := api.Config{
		Peers:               c.registeredNodes,
		Role:                role,
		DesignatedSender:    c.designatedSender,
		T:                   c.t,
		D:                   c.d,
		PublicCryptoParams:  c.publicCryptoParams,
		PrivateCryptoParams: c.privateCryptoParams[node.ID],
		Algorithm:           c.algorithm,
		PayloadSize:         c.payloadSize,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(config)
}

func (c *Controller) handleReady(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	if _, ok := c.registeredNodes[id]; !ok {
		c.mu.Unlock()
		http.Error(w, "Node not registered", http.StatusForbidden)
		return
	}

	if !c.readyNodes[id] {
		c.readyNodes[id] = true
		fmt.Printf("Node %d is ready\n", id)

		if len(c.readyNodes) == c.targetNodes {
			fmt.Println("All nodes ready. Unblocking.")
			fmt.Printf("Starting algorithm %s with %d nodes, t=%d, d=%d\n", c.algorithm, c.targetNodes, c.t, c.d)
			close(c.allReady)
		}
	}

	allReady := len(c.readyNodes) == c.targetNodes
	c.mu.Unlock()

	// Block until all nodes ready
	if !allReady {
		<-c.allReady
	}
	w.WriteHeader(http.StatusOK)
}

func SetupBLSThreshold(n int, t int) (map[int][]byte, []byte, map[int][]byte, error) {
	tau := (n+t)/2 + 1

	poly := make([]fr.Element, tau)
	for i := range tau {
		poly[i].SetRandom()
	}

	var masterSecret big.Int
	poly[0].BigInt(&masterSecret)

	var masterPubKey bn254.G2Affine
	_, _, _, G2Gen := bn254.Generators()
	masterPubKey.ScalarMultiplication(&G2Gen, &masterSecret)
	masterPubKeyBytes := masterPubKey.Marshal()

	privateShares := make(map[int][]byte)
	nodePubKeysBytes := make(map[int][]byte)

	for nodeID := range n {
		var x, eval fr.Element

		// nodeIDs are 0-indexed. But polynomial points are 1-indexed
		x.SetUint64(uint64(nodeID + 1))

		// Evaluate polynomial at x using Horner's method
		eval.Set(&poly[tau-1])
		for j := tau - 2; j >= 0; j-- {
			eval.Mul(&eval, &x)
			eval.Add(&eval, &poly[j])
		}

		shareBytes := eval.Bytes()
		privateShares[nodeID] = shareBytes[:]

		var nodePubKey bn254.G2Affine
		var evalBigInt big.Int
		eval.BigInt(&evalBigInt)
		nodePubKey.ScalarMultiplication(&G2Gen, &evalBigInt)
		nodePubKeysBytes[nodeID] = nodePubKey.Marshal()
	}

	return privateShares, masterPubKeyBytes, nodePubKeysBytes, nil
}

func (c *Controller) SetupKZG(n int) ([]byte, []byte) {
	size := uint64(n)

	var alpha fr.Element
	alpha.SetRandom()
	var bigAlpha big.Int
	alpha.BigInt(&bigAlpha)

	srs, err := kzg.NewSRS(size, &bigAlpha)
	if err != nil {
		panic(err)
	}

	pk := srs.Pk
	vk := srs.Vk

	var pkBuf bytes.Buffer
	var vkBuf bytes.Buffer

	if _, err = pk.WriteTo(&pkBuf); err != nil {
		panic(err)
	}
	if _, err = vk.WriteTo(&vkBuf); err != nil {
		panic(err)
	}

	fmt.Println("Successfully generated KZG parameters.")
	return pkBuf.Bytes(), vkBuf.Bytes()
}

func (c *Controller) InitializeCrypto(n int) {
	pkBytes, vkBytes := c.SetupKZG(n)

	blsShares, masterPubKeyBytes, nodePubKeysBytes, err := SetupBLSThreshold(n, c.t)
	if err != nil {
		panic(err)
	}

	k := n - c.t - 2*c.d
	if k <= 0 || k > n {
		panic(fmt.Sprintf("invalid coding params: require 1 <= k <= n and k <= n - t - 2d, got n=%d t=%d d=%d k=%d", n, c.t, c.d, k))
	}

	c.publicCryptoParams = api.PublicCryptoParams{
		NumDataShards:     k,
		NumParityShards:   n - k,
		PKBytes:           pkBytes,
		VKBytes:           vkBytes,
		MasterPubKeyBytes: masterPubKeyBytes,
		NodePubKeysBytes:  nodePubKeysBytes,
	}

	c.privateCryptoParams = make(map[int]api.PrivateCryptoParams)
	for nodeID, shareBytes := range blsShares {
		c.privateCryptoParams[nodeID] = api.PrivateCryptoParams{
			NodeIndex:         nodeID,
			PrivateShareBytes: shareBytes,
		}
	}
}
