package singbox

import (
	"encoding/json"
	"fmt"
)

// Processor errors
var (
	ErrInvalidSingBoxConfig   = fmt.Errorf("invalid SingBox configuration")
	ErrMissingOutbounds       = fmt.Errorf("missing outbounds field in configuration")
	ErrInvalidOutboundsFormat = fmt.Errorf("invalid outbounds field format")
)

// SingBoxProcessor handles SingBox subscription data processing.
// It processes native SingBox format configurations and extracts proxy nodes.
type SingBoxProcessor struct{}

// NewSingBoxProcessor creates a new SingBox processor instance.
// Returns a processor that can handle SingBox format subscription data.
func NewSingBoxProcessor() *SingBoxProcessor {
	return &SingBoxProcessor{}
}

// Process handles SingBox subscription data and preserves all original fields.
// It parses the JSON configuration, extracts outbound proxy configurations,
// and filters out non-proxy entries like direct, block, and selector types.
func (sp *SingBoxProcessor) Process(data []byte) ([]map[string]any, error) {
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSingBoxConfig, err)
	}

	outboundsRaw, ok := config["outbounds"]
	if !ok {
		return nil, ErrMissingOutbounds
	}

	outboundsArray, ok := outboundsRaw.([]any)
	if !ok {
		return nil, ErrInvalidOutboundsFormat
	}

	var nodes []map[string]any
	for _, outboundRaw := range outboundsArray {
		outboundMap, ok := outboundRaw.(map[string]any)
		if !ok {
			continue
		}

		// 检查类型，排除 direct、block、selector、urltest
		outboundType, ok := outboundMap["type"].(string)
		if !ok || outboundType == "direct" || outboundType == "block" || outboundType == "selector" || outboundType == "urltest" {
			continue
		}

		// 直接保留原始节点数据，不做任何修改
		nodes = append(nodes, outboundMap)
	}

	return nodes, nil
}
