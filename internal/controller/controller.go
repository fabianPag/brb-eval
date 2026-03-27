package controller

import (
	"brb-eval/internal/controller/api"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
)

type Controller struct {
	mu              sync.Mutex
	targetNodes     int
	registeredNodes map[int]api.PeerInfo
	readyNodes      map[int]bool
	nodeMetrics     map[int]api.MetricsData
	allRegistered   chan struct{}
	allReady        chan struct{}
}

func New(targetNodes int) *Controller {
	return &Controller{
		targetNodes:     targetNodes,
		registeredNodes: make(map[int]api.PeerInfo),
		readyNodes:      make(map[int]bool),
		nodeMetrics:     make(map[int]api.MetricsData),
		allRegistered:   make(chan struct{}),
		allReady:        make(chan struct{}),
	}
}

func (c *Controller) Run(addr string) error {
	http.HandleFunc("/register", c.handleRegister)
	http.HandleFunc("/ready", c.handleReady)
	http.HandleFunc("/metrics", c.handleMetrics)
	fmt.Println("Starting controller at", addr)
	return http.ListenAndServe(addr, nil)
}

func (c *Controller) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var metrics api.MetricsData
	if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	c.nodeMetrics[metrics.NodeID] = metrics
	receivedCount := len(c.nodeMetrics)
	allReceived := receivedCount == c.targetNodes
	c.mu.Unlock()

	fmt.Printf("Node %d sent metrics\n", metrics.NodeID)

	if allReceived {
		if err := c.saveMetricsToFile("metrics.json"); err != nil {
			fmt.Printf("[ERROR] Failed to save metrics: %v\n", err)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (c *Controller) saveMetricsToFile(filename string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := json.MarshalIndent(c.nodeMetrics, "", "  ")
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

	c.mu.Lock()
	defer c.mu.Unlock()

	config := api.Config{
		Peers:          c.registeredNodes,
		ShouldInitiate: node.ID == 0,
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
