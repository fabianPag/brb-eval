package manager

import (
	"brb-eval/internal/node/core"
)

type SplitManager struct {
	core.Manager
	lowerBound core.NodeID
	upperBound core.NodeID
}

func NewSplitManager(underlying core.Manager, min, max core.NodeID) *SplitManager {
	return &SplitManager{
		Manager:    underlying,
		lowerBound: min,
		upperBound: max,
	}
}

// restrict sends to nodes within the split
func (pm *SplitManager) SendBatch(messages map[core.NodeID][]byte) {
	filtered := make(map[core.NodeID][]byte)
	for to, payload := range messages {
		if to >= pm.lowerBound && to < pm.upperBound {
			filtered[to] = payload
		}
	}
	pm.Manager.SendBatch(filtered)
}

// send only to nodes within split
func (pm *SplitManager) Broadcast(payload []byte) {
	messages := make(map[core.NodeID][]byte)
	for nodeID := pm.lowerBound; nodeID < pm.upperBound; nodeID++ {
		messages[nodeID] = payload
	}
	pm.Manager.SendBatch(messages)
}
