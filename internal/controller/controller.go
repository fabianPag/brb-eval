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

type Options struct {
	N                int
	T                int
	D                int
	Algorithm        string
	PayloadSize      int
	DesignatedSender int
	TwinsSender      bool
	NumSilent        int
	NumMsgDrops      int
	GracePeriod      time.Duration
	KeepControllerAlive        bool
}

type Controller struct {
	mu                  sync.Mutex
	opts                Options
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
}

type metricsFile struct {
	Algorithm        string            `json:"algorithm"`
	PayloadSize      int               `json:"payload_size"`
	N                int               `json:"n"`
	T                int               `json:"t"`
	D                int               `json:"d"`
	DesignatedSender int               `json:"designated_sender"`
	NumSilent        int               `json:"num_silent"`
	NumMsgDrops      int               `json:"num_msg_drops"`
	NodeMetrics      []api.MetricsData `json:"node_metrics"`
}

func New(opts Options) *Controller {
	return &Controller{
		opts: opts,

		nodeMetrics:        make([]api.MetricsData, opts.N),
		registeredNodes:    make(map[int]api.PeerInfo),
		readyNodes:         make(map[int]bool),
		doneNodes:          make(map[int]bool),
		metricsReceived:    make(map[int]bool),
		allRegistered:      make(chan struct{}),
		allReady:           make(chan struct{}),
		allDone:            make(chan struct{}),
		allMetricsReceived: make(chan struct{}),
	}
}

type statusResponse struct {
	AllMetricsReceived   bool `json:"allMetricsReceived"`
	AllNodesDone         bool `json:"allNodesDone"`
	AllNodesReady        bool `json:"allNodesReady"`
	AllNodesRegistered   bool `json:"allNodesRegistered"`
	DoneCount            int  `json:"doneCount"`
	MetricsReceivedCount int  `json:"metricsReceivedCount"`
	ReadyCount           int  `json:"readyCount"`
	RegisteredCount      int  `json:"registeredCount"`
	TargetNodes          int  `json:"targetNodes"`
}

func (c *Controller) handleStatus(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	status := statusResponse{
		AllMetricsReceived:   len(c.metricsReceived) == c.opts.N,
		AllNodesDone:         len(c.doneNodes) == c.opts.N,
		AllNodesReady:        len(c.readyNodes) == c.opts.N,
		AllNodesRegistered:   len(c.registeredNodes) == c.opts.N,
		DoneCount:            len(c.doneNodes),
		MetricsReceivedCount: len(c.metricsReceived),
		ReadyCount:           len(c.readyNodes),
		RegisteredCount:      len(c.registeredNodes),
		TargetNodes:          c.opts.N,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (c *Controller) Run(addr string, shouldSaveMetrics bool) error {
	c.InitializeCrypto()

	http.HandleFunc("/register", c.handleRegister)
	http.HandleFunc("/ready", c.handleReady)
	http.HandleFunc("/done", c.handleDone)
	http.HandleFunc("/metrics", c.handleMetrics)

	// endpoints used for GCP
	http.HandleFunc("/status", c.handleStatus)
	http.HandleFunc("/download/metrics", c.handleDownloadMetrics)

	fmt.Println("Starting controller at", addr)

	go http.ListenAndServe(addr, nil)
	<-c.allMetricsReceived

	if shouldSaveMetrics {
		if err := c.saveMetricsToFile("metrics.json"); err != nil {
			fmt.Printf("[ERROR] Failed to save metrics: %v\n", err)
		}
	}

	// Keep controller alive for GCP
	if c.opts.KeepControllerAlive {
		fmt.Println("Test complete. Keeping controller alive to serve metric downloads...")
		select {}
	} else {
		fmt.Println("Test complete. Exiting controller.")
	}

	return nil
}

func (c *Controller) handleDownloadMetrics(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	output := metricsFile{
		Algorithm:        c.opts.Algorithm,
		PayloadSize:      c.opts.PayloadSize,
		N:                c.opts.N,
		T:                c.opts.T,
		D:                c.opts.D,
		DesignatedSender: c.opts.DesignatedSender,
		NumSilent:        c.opts.NumSilent,
		NumMsgDrops:      c.opts.NumMsgDrops,
		NodeMetrics:      c.nodeMetrics,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(output); err != nil {
		fmt.Printf("[ERROR] Failed to serve metrics payload: %v\n", err)
	}
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

		if len(c.doneNodes) == c.opts.N {
			fmt.Printf("All nodes done. Waiting for %v to let any in-flight algorithm messages arrive.\n", c.opts.GracePeriod)
			go func() {
				time.Sleep(c.opts.GracePeriod)
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

	if metrics.NodeID < 0 || metrics.NodeID >= c.opts.N {
		http.Error(w, "Invalid Node ID", http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	c.nodeMetrics[metrics.NodeID] = metrics
	c.metricsReceived[metrics.NodeID] = true
	allReceived := len(c.metricsReceived) == c.opts.N
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
		Algorithm:        c.opts.Algorithm,
		PayloadSize:      c.opts.PayloadSize,
		N:                c.opts.N,
		T:                c.opts.T,
		D:                c.opts.D,
		DesignatedSender: c.opts.DesignatedSender,
		NumSilent:        c.opts.NumSilent,
		NumMsgDrops:      c.opts.NumMsgDrops,
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
	allRegistered := len(c.registeredNodes) == c.opts.N
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
	if node.ID == c.opts.DesignatedSender {
		if c.opts.TwinsSender {
			role = "twins_sender"
		} else {
			role = "honest"
		}
	} else if int(node.ID) >= c.opts.N-c.opts.NumSilent {
		role = "silent"
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	honestNodes := make([]int, 0)
	for i := 0; i < c.opts.N; i++ {
		if i < c.opts.N-c.opts.NumSilent {
			honestNodes = append(honestNodes, i)
		}
	}

	config := api.Config{
		Peers:               c.registeredNodes,
		Role:                role,
		DesignatedSender:    c.opts.DesignatedSender,
		T:                   c.opts.T,
		D:                   c.opts.D,
		NumMsgDrops:         c.opts.NumMsgDrops,
		HonestNodes:         honestNodes,
		PublicCryptoParams:  c.publicCryptoParams,
		PrivateCryptoParams: c.privateCryptoParams[node.ID],
		Algorithm:           c.opts.Algorithm,
		PayloadSize:         c.opts.PayloadSize,
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

		if len(c.readyNodes) == c.opts.N {
			fmt.Println("All nodes ready. Unblocking.")
			fmt.Printf("Starting algorithm %s with %d nodes, t=%d, d=%d\n", c.opts.Algorithm, c.opts.N, c.opts.T, c.opts.D)
			close(c.allReady)
		}
	}

	allReady := len(c.readyNodes) == c.opts.N
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

func (c *Controller) InitializeCrypto() {
	pkBytes, vkBytes := c.SetupKZG(c.opts.N)

	blsShares, masterPubKeyBytes, nodePubKeysBytes, err := SetupBLSThreshold(c.opts.N, c.opts.T)
	if err != nil {
		panic(err)
	}

	k := c.opts.N - c.opts.T - 2*c.opts.D
	if k <= 0 || k > c.opts.N {
		panic(fmt.Sprintf("invalid coding params: require 1 <= k <= n and k <= n - t - 2d, got n=%d t=%d d=%d k=%d", c.opts.N, c.opts.T, c.opts.D, k))
	}

	c.publicCryptoParams = api.PublicCryptoParams{
		NumDataShards:     k,
		NumParityShards:   c.opts.N - k,
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
