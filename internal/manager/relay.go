package manager

import (
	"fmt"
	"strings"

	"node-box/internal/fileops"
	"node-box/internal/logger"
	"node-box/internal/subscription"
	"node-box/internal/utils"
)

// updateRelayDetourForAllTargets expands relay subscription nodes by pairing each
// relay template node with every non-relay node tag as a detour target.
// The results are stored in nm.cache.relayExpanded.
func (nm *NodeManager) updateRelayDetourForAllTargets() error {
	relaySubs := nm.relaySubscriptionNames()
	if len(relaySubs) == 0 {
		return nil
	}

	detourTags := nm.collectNonRelayTags()
	if len(detourTags) == 0 {
		logger.Debug("未找到可用的detour标签，跳过relay处理")
		return nil
	}

	logger.Debug("找到 %d 个可用的detour标签", len(detourTags))

	for _, relaySub := range relaySubs {
		relayNodes, exists := nm.cache.nodes[relaySub]
		if !exists {
			logger.Error("relay订阅 %s 不在缓存中", relaySub)
			continue
		}
		if len(relayNodes) == 0 {
			logger.Debug("relay订阅 %s 没有节点", relaySub)
			continue
		}

		expanded := nm.expandRelayNodes(relaySub, relayNodes, detourTags)
		nm.cache.relayExpanded["relay:"+relaySub] = expanded
	}

	return nil
}

// expandRelayNodes creates one copy of each relay node per detour tag,
// setting the detour field and making the tag unique.
func (nm *NodeManager) expandRelayNodes(relaySub string, relayNodes []subscription.Node, detourTags []string) []subscription.Node {
	prefix := fmt.Sprintf("[%s] ", relaySub)
	var expanded []subscription.Node

	for _, n := range relayNodes {
		base := map[string]any(n)
		baseTag, _ := base["tag"].(string)

		originalName := baseTag
		if strings.HasPrefix(baseTag, prefix) {
			originalName = strings.TrimPrefix(baseTag, prefix)
		}

		for _, detour := range detourTags {
			if detour == "" {
				continue
			}
			nm2 := utils.CloneMap(base)
			nm2["detour"] = detour
			nm2["tag"] = fmt.Sprintf("[%s] %s %s", relaySub, originalName, detour)
			expanded = append(expanded, subscription.Node(nm2))
		}
	}

	return expanded
}

// writeRelayNodesToOutbounds writes the expanded relay nodes into outbound module files.
// Steps:
//  1. Filter relay nodes by relay_nodes rules and target subscriptions
//  2. Insert matching nodes into the module file
//  3. Optionally update selector outbounds based on include_relay_nodes
func (nm *NodeManager) writeRelayNodesToOutbounds() error {
	logger.Debug("开始将 relay 节点写入出站模块配置...")

	if nm.config.Nodes == nil || len(nm.config.Nodes.RelayNodes) == 0 {
		logger.Debug("未配置 relay_nodes，跳过 relay 节点写入")
		return nil
	}

	if nm.config.Modules == nil || len(nm.config.Modules.Outbounds) == 0 {
		return nil
	}

	for _, mod := range nm.config.Modules.Outbounds {
		if mod.Path == "" || len(mod.Selectors) == 0 {
			continue
		}

		relayNodesToWrite := nm.filterRelayNodesByIncludeAndSubscriptions(mod.Subscriptions)
		if len(relayNodesToWrite) == 0 {
			logger.Debug("目标 %s: 没有符合 relay_nodes 和 subscriptions 条件的 relay 节点", mod.Path)
			continue
		}

		for _, selector := range mod.Selectors {
			if err := nm.writeNodesToConfigFile(mod.Path, selector.InsertMarker, relayNodesToWrite); err != nil {
				return fmt.Errorf("写入节点到模块文件失败 %s: %v", mod.Path, err)
			}

			if len(selector.IncludeRelayNodes) > 0 {
				if err := nm.updateSelectorForRelayNodes(mod.Path, selector.InsertMarker, relayNodesToWrite, selector.IncludeRelayNodes); err != nil {
					return fmt.Errorf("更新 selector 失败 %s: %v", mod.Path, err)
				}
				logger.Debug("模块文件 %s, 选择器 %s: 写入 %d 个 relay 节点，并更新 selector", mod.Path, selector.InsertMarker, len(relayNodesToWrite))
			} else {
				logger.Debug("模块文件 %s, 选择器 %s: 写入 %d 个 relay 节点，未配置 include_relay_nodes", mod.Path, selector.InsertMarker, len(relayNodesToWrite))
			}
		}
	}

	logger.Debug("relay 节点写入配置完成")
	return nil
}

// filterRelayNodesByIncludeAndSubscriptions filters expanded relay nodes by:
//  1. Whether the node belongs to one of the target subscriptions (if specified)
//  2. Whether the node matches any relay_nodes rule (tag + upstream keywords)
func (nm *NodeManager) filterRelayNodesByIncludeAndSubscriptions(targetSubscriptions []string) []subscription.Node {
	var targetSet map[string]bool
	if len(targetSubscriptions) > 0 {
		targetSet = make(map[string]bool, len(targetSubscriptions))
		for _, name := range targetSubscriptions {
			targetSet[name] = true
		}
	}

	var result []subscription.Node
	for _, nodes := range nm.cache.relayExpanded {
		for _, node := range nodes {
			tag, ok := node["tag"].(string)
			if !ok || tag == "" {
				continue
			}

			if targetSet != nil && !nm.tagBelongsToAnySubscription(tag, targetSet) {
				continue
			}

			if nm.matchesRelayNodeRule(tag) {
				result = append(result, node)
			}
		}
	}

	if len(targetSubscriptions) > 0 {
		logger.Debug("根据 subscriptions %v 和 relay_nodes 筛选出 %d 个节点", targetSubscriptions, len(result))
	} else {
		logger.Debug("根据 relay_nodes 筛选出 %d 个节点", len(result))
	}
	return result
}

// matchesRelayNodeRule reports whether tag satisfies any configured relay_nodes rule.
func (nm *NodeManager) matchesRelayNodeRule(tag string) bool {
	for _, rule := range nm.config.Nodes.RelayNodes {
		if rule.Tag == "" || len(rule.Upstream) == 0 {
			continue
		}
		if !utils.ContainsIgnoreEmoji(tag, rule.Tag) {
			continue
		}
		for _, up := range rule.Upstream {
			if up != "" && utils.ContainsIgnoreEmoji(tag, up) {
				return true
			}
		}
	}
	return false
}

// tagBelongsToAnySubscription reports whether tag contains "[subName]" for any name in the set.
func (nm *NodeManager) tagBelongsToAnySubscription(tag string, subSet map[string]bool) bool {
	for subName := range subSet {
		if strings.Contains(tag, fmt.Sprintf("[%s]", subName)) {
			return true
		}
	}
	return false
}

// writeNodesToConfigFile inserts relay nodes into the specified config file.
func (nm *NodeManager) writeNodesToConfigFile(configPath, insertMarker string, nodes []subscription.Node) error {
	nodesMaps := nodesToMaps(nodes)
	updater := fileops.NewUpdater(insertMarker)
	return updater.InsertRealNodes(configPath, nodesMaps, nm.relaySubscriptionNames())
}

// updateSelectorForRelayNodes updates the selector's outbounds list using relayNodes
// as include keywords to filter which relay node tags are added.
func (nm *NodeManager) updateSelectorForRelayNodes(configPath, insertMarker string, nodes []subscription.Node, relayNodes []string) error {
	nodesMaps := nodesToMaps(nodes)
	updater := fileops.NewUpdater(insertMarker)
	return updater.UpdateSelectorOnly(configPath, nodesMaps, nm.relaySubscriptionNames(), relayNodes, nil)
}

// relaySubscriptionNames returns the names of all enabled relay-type subscriptions.
func (nm *NodeManager) relaySubscriptionNames() []string {
	var names []string
	for _, sub := range nm.config.Nodes.Subscriptions {
		if sub.Enable && strings.ToLower(sub.Type) == "relay" {
			names = append(names, sub.Name)
		}
	}
	return names
}

// collectNonRelayTags collects node tags from all enabled non-relay subscriptions,
// preserving the order defined in the config.
func (nm *NodeManager) collectNonRelayTags() []string {
	var tags []string
	for _, sub := range nm.config.Nodes.Subscriptions {
		if !sub.Enable || strings.ToLower(sub.Type) == "relay" {
			continue
		}
		if nodes, exists := nm.cache.nodes[sub.Name]; exists {
			for _, node := range nodes {
				if tag, ok := node["tag"].(string); ok && tag != "" {
					tags = append(tags, tag)
				}
			}
		}
	}
	return tags
}
