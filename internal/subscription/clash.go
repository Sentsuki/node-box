package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

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
	singboxOutbounds, singboxEndpoints, err := convert.Clash2sing(clashConfig, cp.version)
	if err != nil {
		// 逐条输出转换警告，跳过失败的节点
		for _, line := range strings.Split(err.Error(), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				logger.Warn("Conversion skipped: %s", line)
			}
		}
	}

	var nodes []Node

	// 处理常规 outbounds（VMess, VLESS, SS 等）
	for _, singboxNode := range singboxOutbounds {
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

	// 处理 endpoints（WireGuard 等），统一放入 nodes 中
	// 后续 updater.go 的 moveSpecialOutboundsToEndpoints 会自动将
	// type 为 wireguard 的节点从 outbounds 移动到 endpoints
	for _, ep := range singboxEndpoints {
		nodeBytes, err := json.Marshal(ep)
		if err != nil {
			logger.Error("Failed to marshal endpoint %s: %v", ep.Tag, err)
			continue
		}

		var node Node
		if err := json.Unmarshal(nodeBytes, &node); err != nil {
			logger.Error("Failed to unmarshal endpoint %s: %v", ep.Tag, err)
			continue
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}
