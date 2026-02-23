package fault

import "brb-eval/core"

type Injector interface {
	ProcessSend(to core.NodeID, payload []byte, transport core.Transport) error
}

type PassThroughInjector struct{}

func (p *PassThroughInjector) ProcessSend(to core.NodeID, payload []byte, transport core.Transport) error {
	return transport.Send(to, payload)
}