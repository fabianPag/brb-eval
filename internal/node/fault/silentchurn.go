package fault

import (
	"brb-eval/internal/node/core"
	"fmt"
)

type ChurnInjector struct {
	dropped map[core.NodeID]bool
}

func NewChurnInjector(dropped map[core.NodeID]bool) *ChurnInjector {
	return &ChurnInjector{dropped: dropped}
}

func (c *ChurnInjector) ProcessSend(to core.NodeID, payload []byte, transport core.Transport) error {
	if c.dropped[to] {
		fmt.Printf("[ADVERSARY] Silently dropping message to node %d\n", to)
		return nil
	}
	return transport.Send(to, payload)
}
