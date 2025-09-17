package subscription

import (
	"encoding/json"
	"fmt"

	"node-box/internal/logger"
	"node-box/internal/subscription/clash/convert"
	"node-box/internal/subscription/clash/model"
	"node-box/internal/subscription/clash/model/clash"

	"gopkg.in/yaml.v3"
)

// ClashProcessor handles Clash subscription data processing.
// It converts Clash format proxy configurations to SingBox format nodes using the new conversion logic.
type ClashProcessor struct {
	version model.SingBoxVer
}

// NewClashProcessor creates a new Clash processor instance.
// Returns a processor that can handle Clash format subscription data.
func NewClashProcessor() *ClashProcessor {
	return &ClashProcessor{
		version: model.SINGLATEST, // 使用最新版本
	}
}

// Process handles Clash subscription data and converts it to SingBox format nodes.
// It parses the YAML data, extracts proxy configurations, and converts each
// proxy to the unified Node format compatible with SingBox using the new conversion logic.
func (cp *ClashProcessor) Process(data []byte) ([]Node, error) {
	var clashConfig clash.Clash
	if err := yaml.Unmarshal(data, &clashConfig); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidClashConfig, err)
	}

	// 使用新的转换逻辑
	singboxNodes, err := convert.Clash2sing(clashConfig, cp.version)
	if err != nil {
		logger.Warn("Conversion warnings: %v", err)
		// 继续处理，因为可能只是部分节点转换失败
	}

	var nodes []Node
	for _, singboxNode := range singboxNodes {
		// 将SingBoxOut转换为Node (map[string]any)
		nodeBytes, err := json.Marshal(singboxNode)
		if err != nil {
			logger.Error("Failed to marshal node %s: %v", singboxNode.Tag, err)
			continue
		}

		var node Node
		if err := json.Unmarshal(nodeBytes, &node); err != nil {
			logger.Error("Failed to unmarshal node %s: %v", singboxNode.Tag, err)
			continue
		}

		// 过滤掉被忽略的节点
		if singboxNode.Ignored {
			continue
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}
