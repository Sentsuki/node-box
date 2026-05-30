package manager

import (
	"fmt"
	"strings"

	"node-box/internal/fileops"
	"node-box/internal/logger"
	"node-box/internal/subscription"
)

// UpdateOutboundsConfigs updates outbounds module files with new proxy nodes.
// Steps:
//  1. Find outbound modules that have a local path configured
//  2. Filter nodes by global exclude_keywords
//  3. Insert real nodes into the module file
//  4. Update selectors according to each selector rule
func (nm *NodeManager) UpdateOutboundsConfigs() error {
	logger.Debug("开始更新出站模块(outbounds)文件节点")

	if nm.config.Modules == nil || len(nm.config.Modules.Outbounds) == 0 {
		logger.Debug("没有配置 outbounds 模块，跳过出站节点更新")
		return nil
	}

	if !nm.cache.valid {
		if err := nm.FetchAndCacheAllSubscriptions(); err != nil {
			logger.Warn("获取订阅数据时出现问题: %v，但继续处理", err)
		}
	}

	var updateErrors []string
	totalSuccessCount := 0
	totalFileCount := 0

	for _, mod := range nm.config.Modules.Outbounds {
		if mod.Path == "" {
			continue
		}

		if len(mod.Selectors) == 0 && len(mod.Subscriptions) == 0 {
			logger.Debug("出站模块 %s 没有配置 proxy 规则，跳过", mod.Path)
			continue
		}

		totalFileCount++
		logger.Debug("处理出站模块: %s (%s)", mod.Name, mod.Path)

		uniqueSubs := dedup(mod.Subscriptions)

		allTargetNodes, err := nm.FetchNodesFromSubscriptions(uniqueSubs)
		if err != nil {
			errMsg := fmt.Sprintf("获取节点失败 %s: %v", mod.Path, err)
			logger.Error("%s", errMsg)
			updateErrors = append(updateErrors, errMsg)
			continue
		}

		nonRelayNodes := nm.filterOutRelayNodes(allTargetNodes)
		if len(nonRelayNodes) == 0 {
			logger.Debug("路径 %s 未获取到非relay节点，跳过", mod.Path)
			continue
		}

		filteredNodes := nm.filter.FilterNodes(nonRelayNodes)
		logger.Debug("节点过滤: %d -> %d (排除 %d)", len(nonRelayNodes), len(filteredNodes), len(nonRelayNodes)-len(filteredNodes))

		nodesMaps := nodesToMaps(filteredNodes)

		if len(mod.Selectors) > 0 {
			updater := fileops.NewUpdater("")
			if err := updater.CleanAllSubscriptionArtifacts(mod.Path); err != nil {
				errMsg := fmt.Sprintf("清理订阅残留失败 %s: %v", mod.Path, err)
				logger.Error("%s", errMsg)
				updateErrors = append(updateErrors, errMsg)
				continue
			}
			if err := updater.InsertRealNodes(mod.Path, nodesMaps, uniqueSubs); err != nil {
				errMsg := fmt.Sprintf("插入节点失败 %s: %v", mod.Path, err)
				logger.Error("%s", errMsg)
				updateErrors = append(updateErrors, errMsg)
				continue
			}
		}

		for _, selectorRule := range mod.Selectors {
			updater := fileops.NewUpdater(selectorRule.InsertMarker)
			if err := updater.UpdateSelectorOnly(mod.Path, nodesMaps, uniqueSubs, selectorRule.IncludeNodes, selectorRule.ExcludeNodes); err != nil {
				errMsg := fmt.Sprintf("更新selector失败 %s: %v", mod.Path, err)
				logger.Error("%s", errMsg)
				updateErrors = append(updateErrors, errMsg)
			}
		}

		logger.Debug("路径 %s 处理完成", mod.Path)
		totalSuccessCount++
	}

	if len(updateErrors) > 0 {
		if totalSuccessCount > 0 {
			return fmt.Errorf("%w: %d successful, %d failed", ErrPartialUpdateFailure, totalSuccessCount, len(updateErrors))
		}
		return fmt.Errorf("%w: %v", ErrAllUpdatesFailure, updateErrors)
	}

	if totalFileCount == 0 {
		logger.Debug("全部出站模块中无匹配 proxy 规则，跳过注入")
		return nil
	}

	logger.Info("节点更新完成: %d 个出站模块", totalFileCount)
	return nil
}

// UpdateModuleConfigs fetches all configured modules and assembles the target config files.
func (nm *NodeManager) UpdateModuleConfigs() error {
	if nm.config.Modules == nil || len(nm.config.Configs) == 0 {
		logger.Debug("没有配置模块或配置文件，跳过模块配置更新")
		return nil
	}

	logger.Debug("开始更新模块配置...")

	if err := nm.moduleManager.FetchAllModules(); err != nil {
		logger.Warn("获取模块时出现问题: %v，但继续处理", err)
	}

	var updateErrors []string
	successCount := 0

	for _, configFile := range nm.config.Configs {
		logger.Debug("更新配置文件: %s (%s)", configFile.Name, configFile.Path)

		if err := nm.configUpdater.UpdateConfigFile(configFile); err != nil {
			errMsg := fmt.Sprintf("更新配置文件失败 %s: %v", configFile.Name, err)
			logger.Error("%s", errMsg)
			updateErrors = append(updateErrors, errMsg)
			continue
		}

		successCount++
		logger.Debug("成功更新配置文件: %s", configFile.Name)
	}

	if len(updateErrors) > 0 {
		logger.Info("模块处理完成: 成功 %d 个，失败 %d 个", successCount, len(updateErrors))
		for _, msg := range updateErrors {
			logger.Debug("  - %s", msg)
		}
		if successCount > 0 {
			return fmt.Errorf("部分模块配置更新失败: %d 成功, %d 失败", successCount, len(updateErrors))
		}
		return fmt.Errorf("所有模块配置更新失败: %v", updateErrors)
	}

	if successCount == 0 {
		return fmt.Errorf("没有配置文件需要更新")
	}

	logger.Info("模块处理完成: 成功 %d 个", successCount)
	return nil
}

// filterOutRelayNodes removes nodes that originate from relay-type subscriptions.
// Relay nodes are used only as templates and must not be written directly to config files.
func (nm *NodeManager) filterOutRelayNodes(nodes []subscription.Node) []subscription.Node {
	var result []subscription.Node
	for _, node := range nodes {
		tag, ok := node["tag"].(string)
		if !ok {
			result = append(result, node)
			continue
		}
		if !nm.isRelayNode(tag) {
			result = append(result, node)
		}
	}
	return result
}

// isRelayNode reports whether the given tag belongs to a relay-type subscription.
func (nm *NodeManager) isRelayNode(tag string) bool {
	for _, sub := range nm.config.Nodes.Subscriptions {
		if sub.Enable && strings.ToLower(sub.Type) == "relay" {
			if strings.Contains(tag, fmt.Sprintf("[%s]", sub.Name)) {
				return true
			}
		}
	}
	return false
}

// dedup returns a deduplicated copy of the input slice, preserving order.
func dedup(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// nodesToMaps converts a []subscription.Node slice to []map[string]any.
func nodesToMaps(nodes []subscription.Node) []map[string]any {
	maps := make([]map[string]any, len(nodes))
	for i, n := range nodes {
		maps[i] = map[string]any(n)
	}
	return maps
}
