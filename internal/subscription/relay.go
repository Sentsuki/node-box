package subscription

import (
	"fmt"
	"log"
	"strings"

	"node-box/internal/config"

	"gopkg.in/yaml.v3"
)

// RelayProcessor handles relay subscription data processing.
// It processes relay format proxy configurations and adds detour fields
// based on the relay configuration settings.
type RelayProcessor struct {
	relayConfig *config.RelayConfig
	nodeManager NodeManagerInterface // 用于获取其他订阅的节点
}

// NodeManagerInterface defines the interface for accessing other subscription nodes
type NodeManagerInterface interface {
	FetchNodesFromSubscriptions(subscriptionNames []string) ([]Node, error)
}

// NewRelayProcessor creates a new relay processor instance.
// Returns a processor that can handle relay format subscription data.
func NewRelayProcessor(relayConfig *config.RelayConfig, nodeManager NodeManagerInterface) *RelayProcessor {
	return &RelayProcessor{
		relayConfig: relayConfig,
		nodeManager: nodeManager,
	}
}

// Process handles relay subscription data and adds detour fields to nodes.
// It parses the YAML data, extracts proxy configurations, and adds detour
// fields based on the relay configuration settings.
func (rp *RelayProcessor) Process(data []byte) ([]Node, error) {
	// 首先尝试解析为 Clash 格式
	var clashConfig map[string]interface{}
	if err := yaml.Unmarshal(data, &clashConfig); err != nil {
		return nil, fmt.Errorf("failed to parse relay subscription as YAML: %v", err)
	}

	// 提取 proxies 字段
	proxiesInterface, exists := clashConfig["proxies"]
	if !exists {
		return nil, fmt.Errorf("no proxies field found in relay subscription")
	}

	proxies, ok := proxiesInterface.([]interface{})
	if !ok {
		return nil, fmt.Errorf("proxies field is not an array")
	}

	// 获取所有非 relay 订阅的节点用于匹配 detour
	detourNodes, err := rp.getDetourNodes()
	if err != nil {
		log.Printf("Warning: failed to get detour nodes: %v", err)
		detourNodes = []Node{} // 继续处理，但没有 detour 节点
	}

	var nodes []Node
	for _, proxyInterface := range proxies {
		proxy, ok := proxyInterface.(map[string]interface{})
		if !ok {
			log.Printf("Warning: skipping invalid proxy entry")
			continue
		}

		// 转换为 Node 格式
		node := Node(proxy)

		// 为 relay 节点添加 detour 字段
		if err := rp.addDetourToNode(node, detourNodes); err != nil {
			log.Printf("Warning: failed to add detour to node %v: %v", node["name"], err)
		}

		nodes = append(nodes, node)
	}

	log.Printf("Processed %d relay nodes", len(nodes))
	return nodes, nil
}

// getDetourNodes 获取所有非 relay 订阅的节点
func (rp *RelayProcessor) getDetourNodes() ([]Node, error) {
	if rp.nodeManager == nil {
		return nil, fmt.Errorf("node manager not available")
	}

	// 获取所有订阅的节点
	allNodes, err := rp.nodeManager.FetchNodesFromSubscriptions(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch nodes: %v", err)
	}

	// 过滤出非 relay 类型的节点
	var detourNodes []Node
	for _, node := range allNodes {
		// 检查节点是否来自 relay 订阅
		// 这里假设节点的 tag 包含订阅前缀，格式为 "订阅名_节点名"
		if tag, exists := node["tag"]; exists {
			if tagStr, ok := tag.(string); ok {
				// 如果 tag 不包含 relay 订阅的前缀，则认为是非 relay 节点
				if !rp.isFromRelaySubscription(tagStr) {
					detourNodes = append(detourNodes, node)
				}
			}
		}
	}

	return detourNodes, nil
}

// isFromRelaySubscription 检查节点是否来自 relay 订阅
func (rp *RelayProcessor) isFromRelaySubscription(tag string) bool {
	// 这里需要根据实际的订阅命名规则来判断
	// 假设 relay 订阅的节点 tag 包含特定的前缀或模式
	// 这个逻辑可能需要根据实际情况调整
	return strings.Contains(strings.ToLower(tag), "relay")
}

// addDetourToNode 为节点添加 detour 字段
func (rp *RelayProcessor) addDetourToNode(node Node, detourNodes []Node) error {
	if rp.relayConfig == nil || len(rp.relayConfig.RelayNode) == 0 {
		return nil // 没有配置，跳过
	}

	// 获取节点名称
	nodeName, exists := node["name"]
	if !exists {
		return fmt.Errorf("node has no name field")
	}

	nodeNameStr, ok := nodeName.(string)
	if !ok {
		return fmt.Errorf("node name is not a string")
	}

	// 查找匹配的 relay 配置
	var matchedConfig *config.RelayNodeConfig
	for _, relayNodeConfig := range rp.relayConfig.RelayNode {
		if relayNodeConfig.Name == "wg" { // 根据配置示例，这里匹配 "wg"
			matchedConfig = &relayNodeConfig
			break
		}
	}

	if matchedConfig == nil {
		return nil // 没有匹配的配置
	}

	// 根据 include_keywords 查找匹配的 detour 节点
	var detourTags []string
	for _, detourNode := range detourNodes {
		if tag, exists := detourNode["tag"]; exists {
			if tagStr, ok := tag.(string); ok {
				// 检查是否匹配 include_keywords
				for _, keyword := range matchedConfig.IncludeKeywords {
					if strings.Contains(tagStr, keyword) {
						detourTags = append(detourTags, tagStr)
						break
					}
				}
			}
		}
	}

	// 添加 detour 字段
	if len(detourTags) > 0 {
		// 如果有多个匹配的节点，可以选择第一个或者用其他逻辑
		node["detour"] = detourTags[0]
		log.Printf("Added detour '%s' to relay node '%s'", detourTags[0], nodeNameStr)
	} else {
		// 如果没有匹配的节点，设置空字符串
		node["detour"] = ""
		log.Printf("No matching detour found for relay node '%s'", nodeNameStr)
	}

	return nil
}
