package subscription

import (
	"node-box/internal/subscription/clash"
	"node-box/internal/subscription/singbox"
	"node-box/internal/subscription/xray"
)

// NewClashProcessor creates a Clash processor wrapped as a Processor.
func NewClashProcessor() Processor {
	return &mapAdapter{inner: clash.NewClashProcessor()}
}

// NewSingBoxProcessor creates a SingBox processor wrapped as a Processor.
func NewSingBoxProcessor() Processor {
	return &mapAdapter{inner: singbox.NewSingBoxProcessor()}
}

// NewXrayProcessor creates an Xray processor wrapped as a Processor.
func NewXrayProcessor() Processor {
	return &mapAdapter{inner: xray.NewXrayProcessor()}
}

// subProcessor is the interface that all subpackage processors implement.
type subProcessor interface {
	Process(data []byte) ([]map[string]any, error)
}

// mapAdapter wraps a subpackage processor to convert []map[string]any to []Node.
type mapAdapter struct {
	inner subProcessor
}

func (a *mapAdapter) Process(data []byte) ([]Node, error) {
	raw, err := a.inner.Process(data)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, len(raw))
	for i, n := range raw {
		nodes[i] = Node(n)
	}
	return nodes, nil
}
