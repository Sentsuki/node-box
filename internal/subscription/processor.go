package subscription

import (
	"fmt"
	"strings"

	"node-box/internal/subscription/xray"
)

// NewProcessor 根据订阅类型创建相应的处理器
func NewProcessor(subType string) (Processor, error) {
	switch strings.ToLower(subType) {
	case "clash":
		return NewClashProcessor(), nil
	case "singbox":
		return NewSingBoxProcessor(), nil
	case "relay":
		// relay 类型与 singbox 完全一致，使用相同的处理器
		return NewSingBoxProcessor(), nil
	case "xray", "v2ray":
		return &xrayAdapter{inner: xray.NewXrayProcessor()}, nil
	default:
		return nil, fmt.Errorf("不支持的订阅类型: %s", subType)
	}
}

// xrayAdapter wraps XrayProcessor to convert []map[string]any to []Node.
type xrayAdapter struct {
	inner *xray.XrayProcessor
}

func (a *xrayAdapter) Process(data []byte) ([]Node, error) {
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

