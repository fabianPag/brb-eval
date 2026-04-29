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
func (pm *SplitManager) Send(to core.NodeID, payload []byte) {
	if to >= pm.lowerBound && to < pm.upperBound {
		pm.Manager.Send(to, payload)
	}
}

// send only to nodes within split
func (pm *SplitManager) Broadcast(payload []byte) {
	for nodeID := pm.lowerBound; nodeID < pm.upperBound; nodeID++ {
		go pm.Send(nodeID, payload)
	}
}
