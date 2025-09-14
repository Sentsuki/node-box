package subscription

import (
	"encoding/json"
	"fmt"
)

// SingBoxProcessor SingBox订阅处理器
type SingBoxProcessor struct{}

// NewSingBoxProcessor 创建新的SingBox处理器
func NewSingBoxProcessor() *SingBoxProcessor {
	return &SingBoxProcessor{}
}

// Process 处理SingBox订阅数据，保留所有原始字段
func (sp *SingBoxProcessor) Process(data []byte) ([]Node, error) {
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析SingBox配置失败: %v", err)
	}

	outboundsRaw, ok := config["outbounds"]
	if !ok {
		return nil, fmt.Errorf("配置中缺少 outbounds 字段")
	}

	outboundsArray, ok := outboundsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("outbounds 字段格式错误")
	}

	var nodes []Node
	for _, outboundRaw := range outboundsArray {
		outboundMap, ok := outboundRaw.(map[string]any)
		if !ok {
			continue
		}

		// 检查类型，排除 direct、block、selector
		outboundType, ok := outboundMap["type"].(string)
		if !ok || outboundType == "direct" || outboundType == "block" || outboundType == "selector" {
			continue
		}

		// 直接保留原始节点数据，不做任何修改
		nodes = append(nodes, outboundMap)
	}

	return nodes, nil
}
