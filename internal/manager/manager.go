// Package manager provides the core node management functionality
// that coordinates all other components to manage subscription nodes.
package manager

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"node-box/internal/client"
	"node-box/internal/config"
	"node-box/internal/fileops"
	"node-box/internal/logger"
	"node-box/internal/modules"
	"node-box/internal/subscription"
)

// Manager package errors
var (
	ErrHTTPClientCreation   = errors.New("failed to create HTTP client")
	ErrNoConfigFiles        = errors.New("no configuration files found")
	ErrNoNodes              = errors.New("no nodes retrieved from subscriptions")
	ErrUnsupportedSubType   = errors.New("unsupported subscription type")
	ErrPartialUpdateFailure = errors.New("partial configuration update failure")
	ErrAllUpdatesFailure    = errors.New("all configuration updates failed")
)

// SubscriptionCache holds cached subscription data
type SubscriptionCache struct {
	nodes map[string][]subscription.Node // 订阅名称 -> 节点列表
	valid bool                           // 缓存是否有效
	// 保存relay展开后的节点，key为配置文件路径
	relayExpanded map[string][]subscription.Node
}

// NodeManager coordinates all components to implement core business logic.
// It manages the complete workflow of fetching subscriptions, processing nodes,
// and updating configuration files with caching support.
type NodeManager struct {
	config        *config.Config
	fetcher       *client.Fetcher
	processors    map[string]subscription.Processor
	scanners      map[string]*fileops.Scanner
	filter        *subscription.Filter
	moduleManager *modules.ModuleManager
	configUpdater *modules.ConfigUpdater
	cache         *SubscriptionCache
}

// NewNodeManager creates a new NodeManager instance with all necessary components.
// It initializes HTTP client, subscription processors, file operations, and node filtering
// based on the provided configuration.
func NewNodeManager(cfg *config.Config) (*NodeManager, error) {
	// 创建HTTP客户端
	httpClient, err := client.NewHTTPClient(cfg.Proxy, cfg.UserAgent)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHTTPClientCreation, err)
	}

	// 创建订阅获取器
	fetcher := client.NewFetcher(httpClient)

	// 创建订阅处理器映射
	processors := make(map[string]subscription.Processor)
	processors["clash"] = subscription.NewClashProcessor()
	processors["singbox"] = subscription.NewSingBoxProcessor()
	processors["relay"] = subscription.NewSingBoxProcessor()

	// 为每个配置路径创建扫描器
	scanners := make(map[string]*fileops.Scanner)

	for _, target := range cfg.Nodes.Targets {
		scanners[target.Path] = fileops.NewScanner(target.Path, target.IsFile)
	}

	// 创建节点过滤器
	filter := subscription.NewFilter(cfg.Nodes.ExcludeKeywords)

	// 创建模块管理器
	moduleManager := modules.NewModuleManager(cfg, fetcher)

	// 创建配置更新器
	configUpdater := modules.NewConfigUpdater(moduleManager)

	return &NodeManager{
		config:        cfg,
		fetcher:       fetcher,
		processors:    processors,
		scanners:      scanners,
		filter:        filter,
		moduleManager: moduleManager,
		configUpdater: configUpdater,
		cache: &SubscriptionCache{
			nodes:         make(map[string][]subscription.Node),
			valid:         false,
			relayExpanded: make(map[string][]subscription.Node),
		},
	}, nil
}

// InvalidateCache invalidates the subscription cache, forcing a fresh fetch on next request.
func (nm *NodeManager) InvalidateCache() {
	nm.cache.valid = false
	nm.cache.nodes = make(map[string][]subscription.Node)
	nm.cache.relayExpanded = make(map[string][]subscription.Node)
	logger.Debug("订阅缓存已失效")
}

// ClearCache completely clears all cached data and resets cache state.
// This method should be called after completing all update processes to free memory.
func (nm *NodeManager) ClearCache() {
	nm.cache.valid = false
	nm.cache.nodes = nil
	nm.cache.relayExpanded = nil
	nm.cache = &SubscriptionCache{
		nodes:         make(map[string][]subscription.Node),
		valid:         false,
		relayExpanded: make(map[string][]subscription.Node),
	}
	logger.Debug("订阅缓存已清除")
}

// ClearAllCaches completely clears both subscription and module caches.
// This method should be called after completing all processes to free memory.
func (nm *NodeManager) ClearAllCaches() {
	nm.ClearCache()
	nm.moduleManager.ClearCache()
	logger.Debug("所有缓存已清除")
}

// Cleanup performs complete cleanup of the NodeManager, releasing all resources.
// This method should be called when the NodeManager is no longer needed to prevent memory leaks.
func (nm *NodeManager) Cleanup() {
	if nm == nil {
		return
	}

	logger.Debug("开始清理 NodeManager 资源...")

	// 清理所有缓存
	nm.ClearAllCaches()

	// 清理引用，帮助 GC
	nm.config = nil
	nm.fetcher = nil
	nm.processors = nil
	nm.scanners = nil
	nm.filter = nil
	nm.moduleManager = nil
	nm.configUpdater = nil
	nm.cache = nil

	logger.Debug("NodeManager 资源清理完成")
}

// FetchAndCacheAllSubscriptions fetches all enabled subscriptions and caches the results.
// This method should be called once per update cycle to populate the cache.
// 缓存原始节点（未经全局过滤），过滤将在使用时进行
func (nm *NodeManager) FetchAndCacheAllSubscriptions() error {
	logger.Info("获取所有订阅节点...")

	// 清空缓存
	nm.cache.nodes = make(map[string][]subscription.Node)
	nm.cache.valid = false

	var fetchErrors []string
	successCount := 0

	for _, sub := range nm.config.Nodes.Subscriptions {
		if !sub.Enable {
			continue
		}

		// 确定要使用的User-Agent
		userAgent := sub.UserAgent
		if userAgent == "" {
			userAgent = nm.config.UserAgent
		}

		logger.Debug("获取订阅: %s", sub.Name)

		// 根据配置选择获取方式
		var data []byte
		var err error

		if sub.URL != "" {
			// 从URL获取订阅数据（带重试和自定义User-Agent）
			data, err = nm.fetcher.FetchSubscriptionWithUserAgent(sub.URL, userAgent)
		} else if sub.Path != "" {
			// 从本地路径读取订阅数据
			data, err = nm.fetcher.FetchSubscriptionFromPath(sub.Path)
		} else {
			err = fmt.Errorf("订阅 %s 既没有配置URL也没有配置Path", sub.Name)
		}

		if err != nil {
			errorMsg := fmt.Sprintf("获取订阅失败 %s: %v", sub.Name, err)
			logger.Error("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}

		// 获取对应的处理器
		processor, ok := nm.processors[strings.ToLower(sub.Type)]
		if !ok {
			errorMsg := fmt.Sprintf("%v: %s (subscription: %s)", ErrUnsupportedSubType, sub.Type, sub.Name)
			logger.Error("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}

		// 处理订阅数据
		nodes, err := processor.Process(data)
		if err != nil {
			errorMsg := fmt.Sprintf("处理订阅失败 %s: %v", sub.Name, err)
			logger.Error("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}

		// 添加订阅前缀（缓存原始节点，不进行全局过滤）
		prefixedNodes := subscription.AddSubscriptionPrefix(nodes, sub.Name)

		// 只缓存非relay订阅的原始节点，relay节点仅作为模板使用
		if strings.ToLower(sub.Type) != "relay" {
			nm.cache.nodes[sub.Name] = prefixedNodes
		} else {
			// relay订阅的节点仅用于模板，不缓存到cache_nodes中
			logger.Debug("relay订阅 %s 的 %d 个节点仅作为模板使用，不缓存", sub.Name, len(prefixedNodes))
		}
		successCount++
		logger.Debug("缓存订阅 %s: %d 个节点", sub.Name, len(prefixedNodes))
	}

	// 标记缓存为有效（即使有部分失败）
	if successCount > 0 {
		nm.cache.valid = true
	}

	if len(fetchErrors) > 0 {
		logger.Warn("订阅获取完成: 成功 %d 个，失败 %d 个", successCount, len(fetchErrors))
		if logger.ParseLevel("DEBUG") <= logger.ParseLevel("INFO") {
			logger.Debug("获取失败的订阅:")
			for _, errMsg := range fetchErrors {
				logger.Debug("  - %s", errMsg)
			}
		}

		if successCount == 0 {
			logger.Warn("所有订阅获取失败，但继续处理")
			return nil // 不返回错误，允许继续处理
		}

		logger.Info("部分订阅获取失败，但继续处理成功的 %d 个订阅", successCount)
		return nil // 不返回错误，允许继续处理
	}

	logger.Info("订阅缓存完成: %d 个订阅", successCount)
	return nil
}

// FetchAllNodes retrieves nodes from all enabled subscriptions.
// It uses cached data if available, otherwise fetches fresh data.
func (nm *NodeManager) FetchAllNodes() ([]subscription.Node, error) {
	return nm.FetchNodesFromSubscriptions(nil)
}

// FetchNodesFromSubscriptions retrieves nodes from specified subscriptions using cache.
// If subscriptionNames is nil or empty, it returns nodes from all cached subscriptions.
// If cache is invalid, it will fetch fresh data first.
// 返回原始节点（未经全局过滤）
func (nm *NodeManager) FetchNodesFromSubscriptions(subscriptionNames []string) ([]subscription.Node, error) {
	// 如果缓存无效，先获取所有订阅
	if !nm.cache.valid {
		if err := nm.FetchAndCacheAllSubscriptions(); err != nil {
			logger.Warn("获取订阅时出现问题: %v，但继续处理", err)
		}
	}

	var allNodes []subscription.Node
	var targetSubscriptions map[string]bool

	// 创建订阅名称映射，用于快速查找
	if len(subscriptionNames) > 0 {
		targetSubscriptions = make(map[string]bool)
		for _, name := range subscriptionNames {
			targetSubscriptions[name] = true
		}
	}

	// 从缓存中获取原始节点
	for _, sub := range nm.config.Nodes.Subscriptions {
		if !sub.Enable {
			continue
		}

		// 如果指定了订阅名称，只处理指定的订阅
		if targetSubscriptions != nil && !targetSubscriptions[sub.Name] {
			continue
		}

		// 从缓存获取原始节点
		if cachedNodes, exists := nm.cache.nodes[sub.Name]; exists {
			allNodes = append(allNodes, cachedNodes...)
		} else {
			// 当订阅类型是 relay 时，不提醒，因为 relay 节点仅作为模板使用，不缓存到 cache.nodes 中
			if strings.ToLower(sub.Type) != "relay" {
				logger.Warn("订阅 %s 不在缓存中", sub.Name)
			}
		}
	}

	return allNodes, nil
}

// 注意: per-proxy include/exclude 仅在写入 selector 标签时应用，
// 不在此处对真实节点集合进行过滤。

// UpdateAllConfigs updates all configuration files with new proxy nodes.
// It coordinates the complete workflow of file scanning, node fetching,
// and configuration updating with comprehensive error handling and caching.
// 执行顺序：1.获取所有节点 2.根据全局exclude_keywords排除节点 3.将真实节点插入path指定的配置中 4.根据proxies里指定的规则更新selector
func (nm *NodeManager) UpdateAllConfigs() error {
	logger.Debug("开始更新配置文件")

	// 1. 获取所有节点
	if !nm.cache.valid {
		if err := nm.FetchAndCacheAllSubscriptions(); err != nil {
			logger.Warn("获取订阅数据时出现问题: %v，但继续处理", err)
		}
	}

	var updateErrors []string
	totalSuccessCount := 0
	totalFileCount := 0

	for _, target := range nm.config.Nodes.Targets {
		logger.Debug("处理路径: %s", target.Path)

		// 扫描配置文件
		scanner := nm.scanners[target.Path]
		configFiles, err := scanner.ScanConfigFiles()
		if err != nil {
			errorMsg := fmt.Sprintf("扫描配置文件失败 %s: %v", target.Path, err)
			logger.Error("%s", errorMsg)
			updateErrors = append(updateErrors, errorMsg)
			continue
		}

		if len(configFiles) == 0 {
			logger.Debug("路径 %s 下未找到配置文件", target.Path)
			continue
		}

		totalFileCount += len(configFiles)

		// 准备订阅名称列表
		var subscriptionNames []string
		if len(target.Subscriptions) > 0 {
			subscriptionNames = target.Subscriptions
		} else {
			for _, sub := range nm.config.Nodes.Subscriptions {
				if sub.Enable {
					subscriptionNames = append(subscriptionNames, sub.Name)
				}
			}
		}

		// 获取节点
		allTargetNodes, err := nm.FetchNodesFromSubscriptions(target.Subscriptions)
		if err != nil {
			errorMsg := fmt.Sprintf("获取节点失败 %s: %v", target.Path, err)
			logger.Error("%s", errorMsg)
			updateErrors = append(updateErrors, errorMsg)
			continue
		}

		// 过滤掉relay订阅的节点，relay节点仅作为模板使用，不写入配置
		var nonRelayNodes []subscription.Node
		for _, node := range allTargetNodes {
			// 检查节点是否来自relay订阅
			isFromRelaySub := false
			for _, sub := range nm.config.Nodes.Subscriptions {
				if sub.Enable && strings.ToLower(sub.Type) == "relay" {
					if tag, ok := node["tag"].(string); ok && strings.Contains(tag, fmt.Sprintf("[%s]", sub.Name)) {
						isFromRelaySub = true
						break
					}
				}
			}
			if !isFromRelaySub {
				nonRelayNodes = append(nonRelayNodes, node)
			}
		}

		if len(nonRelayNodes) == 0 {
			logger.Debug("路径 %s 未获取到非relay节点，跳过", target.Path)
			continue
		}

		// 2. 根据全局exclude_keywords排除节点
		filteredNodes := nm.filter.FilterNodes(nonRelayNodes)
		logger.Debug("节点过滤: %d -> %d (排除 %d)", len(nonRelayNodes), len(filteredNodes), len(nonRelayNodes)-len(filteredNodes))

		// 转换为map格式
		nodesMaps := make([]map[string]any, len(filteredNodes))
		for i, node := range filteredNodes {
			nodesMaps[i] = map[string]any(node)
		}

		// 3. 将真实节点插入配置文件（每个文件只插入一次）
		pathSuccessCount := 0
		for _, configFile := range configFiles {
			if len(target.Proxies) > 0 {
				updater := fileops.NewUpdater("")
				if err := updater.InsertRealNodes(configFile, nodesMaps, subscriptionNames); err != nil {
					errorMsg := fmt.Sprintf("插入节点失败 %s: %v", configFile, err)
					logger.Error("%s", errorMsg)
					updateErrors = append(updateErrors, errorMsg)
					continue
				}
				pathSuccessCount++
			}
		}

		// 4. 根据proxies规则更新selector
		for _, proxyRule := range target.Proxies {
			updater := fileops.NewUpdater(proxyRule.InsertMarker)

			for _, configFile := range configFiles {
				if err := updater.UpdateSelectorOnly(configFile, nodesMaps, subscriptionNames, proxyRule.IncludeKeywords, proxyRule.ExcludeKeywords); err != nil {
					errorMsg := fmt.Sprintf("更新selector失败 %s: %v", configFile, err)
					log.Printf("%s", errorMsg)
					updateErrors = append(updateErrors, errorMsg)
				}
			}
		}

		logger.Debug("路径 %s 处理完成: %d 个文件", target.Path, len(configFiles))
		totalSuccessCount += pathSuccessCount
	}

	// 汇总结果
	if len(updateErrors) > 0 {
		if totalSuccessCount > 0 {
			return fmt.Errorf("%w: %d successful, %d failed", ErrPartialUpdateFailure, totalSuccessCount, len(updateErrors))
		}
		return fmt.Errorf("%w: %v", ErrAllUpdatesFailure, updateErrors)
	}

	if totalFileCount == 0 {
		return fmt.Errorf("%w in all configured paths", ErrNoConfigFiles)
	}

	logger.Info("节点更新完成: %d 个文件", totalFileCount)
	return nil
}

// UpdateModuleConfigs updates configuration files with module data.
// It fetches all configured modules and applies them to the specified configuration files.
// Uses cached module data if available.
func (nm *NodeManager) UpdateModuleConfigs() error {
	if nm.config.Modules == nil || len(nm.config.Configs) == 0 {
		logger.Debug("没有配置模块或配置文件，跳过模块配置更新")
		return nil
	}

	logger.Debug("开始更新模块配置...")

	// 1. 获取所有模块（使用缓存机制，只在需要时请求）
	if err := nm.moduleManager.FetchAllModules(); err != nil {
		logger.Warn("获取模块时出现问题: %v，但继续处理", err)
	}

	// 2. 设置总配置文件数量
	nm.configUpdater.SetTotalCount(len(nm.config.Configs))

	// 3. 更新每个配置文件
	var updateErrors []string
	successCount := 0

	for _, configFile := range nm.config.Configs {
		logger.Debug("更新配置文件: %s (%s)", configFile.Name, configFile.Path)

		if err := nm.configUpdater.UpdateConfigFile(configFile); err != nil {
			errorMsg := fmt.Sprintf("更新配置文件失败 %s: %v", configFile.Name, err)
			logger.Error("%s", errorMsg)
			updateErrors = append(updateErrors, errorMsg)
			continue
		}

		successCount++
		logger.Debug("成功更新配置文件: %s", configFile.Name)
	}

	// 3. 汇总结果
	logger.Info("配置更新完成: 成功 %d 个，失败 %d 个", successCount, len(updateErrors))

	if len(updateErrors) > 0 {
		logger.Debug("更新失败的配置文件:")
		for _, errMsg := range updateErrors {
			logger.Debug("  - %s", errMsg)
		}

		if successCount > 0 {
			return fmt.Errorf("部分模块配置更新失败: %d 成功, %d 失败", successCount, len(updateErrors))
		}

		return fmt.Errorf("所有模块配置更新失败: %v", updateErrors)
	}

	if successCount == 0 {
		return fmt.Errorf("没有配置文件需要更新")
	}

	logger.Debug("所有模块配置更新成功")
	return nil
}

// UpdateAllConfigurations updates all configurations in sequence.
// Execution order: 1. 失效缓存 2. 节点配置 3. relay 订阅后处理 4. relay 节点写入配置 5. 模块配置
func (nm *NodeManager) UpdateAllConfigurations() error {
	logger.Debug("开始更新所有配置...")

	var errors []string

	// 1. 失效缓存，确保获取最新数据
	nm.InvalidateCache()
	nm.moduleManager.InvalidateCache()

	// 1.5 在每次更新开始时，清理所有目标配置中包含订阅名前缀的节点与 selector 标签
	// 这样可以移除因为 enable 状态变化或 targets.subscriptions 调整而被排除的历史内容
	var allSubNames []string
	for _, sub := range nm.config.Nodes.Subscriptions {
		// 不论是否启用，都需要作为清理依据
		allSubNames = append(allSubNames, sub.Name)
	}
	for _, target := range nm.config.Nodes.Targets {
		scanner := nm.scanners[target.Path]
		configFiles, err := scanner.ScanConfigFiles()
		if err != nil {
			errorMsg := fmt.Sprintf("扫描配置文件失败 %s: %v", target.Path, err)
			logger.Error("%s", errorMsg)
			errors = append(errors, errorMsg)
			continue
		}
		for _, cfgPath := range configFiles {
			updater := fileops.NewUpdater("")
			if err := updater.CleanSubscriptionArtifacts(cfgPath, allSubNames); err != nil {
				errorMsg := fmt.Sprintf("清理订阅残留失败 %s: %v", cfgPath, err)
				logger.Error("%s", errorMsg)
				errors = append(errors, errorMsg)
			}
		}
	}

	// 2. 更新节点配置
	logger.Info("步骤 1/4: 更新节点配置...")
	if err := nm.UpdateAllConfigs(); err != nil {
		errorMsg := fmt.Sprintf("节点配置更新失败: %v", err)
		logger.Error("%s", errorMsg)
		errors = append(errors, errorMsg)
	} else {
		logger.Debug("节点配置更新成功")
	}

	// 3. relay 订阅后处理（为节点添加 detour）
	logger.Info("步骤 2/4: 处理 Relay 订阅...")
	if err := nm.updateRelayDetourForAllTargets(); err != nil {
		errorMsg := fmt.Sprintf("Relay 订阅后处理失败: %v", err)
		logger.Error("%s", errorMsg)
		errors = append(errors, errorMsg)
	} else {
		logger.Debug("Relay 订阅后处理完成")
	}

	// 4. 将 relay 节点写入配置
	logger.Debug("将 Relay 节点写入配置...")
	if err := nm.writeRelayNodesToConfig(); err != nil {
		errorMsg := fmt.Sprintf("Relay 节点写入配置失败: %v", err)
		logger.Error("%s", errorMsg)
		errors = append(errors, errorMsg)
	} else {
		logger.Info("Relay 节点配置完成")
	}

	// 5. 更新模块配置
	logger.Info("步骤 3/4: 更新模块配置...")
	if err := nm.UpdateModuleConfigs(); err != nil {
		errorMsg := fmt.Sprintf("模块配置更新失败: %v", err)
		logger.Error("%s", errorMsg)
		errors = append(errors, errorMsg)
	} else {
		logger.Debug("模块配置更新成功")
	}

	// 6. 汇总结果
	var finalErr error
	if len(errors) > 0 {
		logger.Warn("配置更新完成，但有错误:")
		for _, errMsg := range errors {
			logger.Debug("  - %s", errMsg)
		}
		finalErr = fmt.Errorf("配置更新完成，但有 %d 个错误", len(errors))
	} else {
		logger.Debug("所有配置更新成功")
	}

	// 7. 清除所有缓存释放内存
	logger.Info("步骤 4/4: 清除所有缓存...")
	nm.ClearAllCaches()
	logger.Info("*****所有流程完成，缓存已清除*****")

	return finalErr
}

// updateRelayDetourForAllTargets 在更新节点配置后，为 relay 类型订阅的节点添加 detour 字段。
func (nm *NodeManager) updateRelayDetourForAllTargets() error {
	// 收集所有 relay 类型的订阅名称
	var relaySubs []string
	for _, sub := range nm.config.Nodes.Subscriptions {
		if sub.Enable && strings.ToLower(sub.Type) == "relay" {
			relaySubs = append(relaySubs, sub.Name)
		}
	}

	if len(relaySubs) == 0 {
		return nil
	}

	// 从订阅缓存中收集所有非relay节点的tag作为detour候选
	var detourTags []string
	for subName, nodes := range nm.cache.nodes {
		// 跳过relay订阅
		isRelaySub := false
		for _, relaySub := range relaySubs {
			if subName == relaySub {
				isRelaySub = true
				break
			}
		}
		if isRelaySub {
			continue
		}

		// 收集该订阅的节点tag
		for _, node := range nodes {
			if tag, ok := node["tag"].(string); ok && tag != "" {
				detourTags = append(detourTags, tag)
			}
		}
	}

	if len(detourTags) == 0 {
		logger.Debug("未找到可用的detour标签，跳过relay处理")
		return nil
	}

	logger.Debug("找到 %d 个可用的detour标签", len(detourTags))

	// 重新获取relay订阅的原始节点作为模板（因为不再缓存到cache.nodes中）
	cloneMap := func(m map[string]any) map[string]any {
		c := make(map[string]any, len(m))
		for k, v := range m {
			c[k] = v
		}
		return c
	}

	for _, relaySub := range relaySubs {
		// 重新获取relay订阅的原始节点
		relayNodes, err := nm.fetchRelaySubscriptionNodes(relaySub)
		if err != nil {
			logger.Error("获取relay订阅 %s 的节点失败: %v", relaySub, err)
			continue
		}

		if len(relayNodes) == 0 {
			continue
		}

		var expanded []subscription.Node
		for _, n := range relayNodes {
			// subscription.Node 是 map[string]any 的命名类型，需显式转换而非类型断言
			base := map[string]any(n)
			baseTag, _ := base["tag"].(string)
			// 提取原始tag中订阅前缀后的部分，例如从 "[normal] US" 提取 "US"
			originalNodeName := baseTag
			if strings.HasPrefix(baseTag, fmt.Sprintf("[%s] ", relaySub)) {
				originalNodeName = strings.TrimPrefix(baseTag, fmt.Sprintf("[%s] ", relaySub))
			}

			for _, detour := range detourTags {
				if detour == "" {
					continue
				}
				nm2 := cloneMap(base)
				nm2["detour"] = detour
				// 组合格式: [relaySub] originalNodeName detourTag
				nm2["tag"] = fmt.Sprintf("[%s] %s %s", relaySub, originalNodeName, detour)
				expanded = append(expanded, subscription.Node(nm2))
			}
		}

		nm.cache.relayExpanded["relay:"+relaySub] = expanded
	}

	return nil
}

// fetchRelaySubscriptionNodes 重新获取指定relay订阅的原始节点作为模板
func (nm *NodeManager) fetchRelaySubscriptionNodes(subName string) ([]subscription.Node, error) {
	// 查找订阅配置
	var subConfig *config.Subscription
	for _, sub := range nm.config.Nodes.Subscriptions {
		if sub.Name == subName && sub.Enable && strings.ToLower(sub.Type) == "relay" {
			subConfig = &sub
			break
		}
	}

	if subConfig == nil {
		return nil, fmt.Errorf("未找到relay订阅: %s", subName)
	}

	// 确定要使用的User-Agent
	userAgent := subConfig.UserAgent
	if userAgent == "" {
		userAgent = nm.config.UserAgent
	}

	// 根据配置选择获取方式
	var data []byte
	var err error

	if subConfig.URL != "" {
		// 从URL获取订阅数据
		data, err = nm.fetcher.FetchSubscriptionWithUserAgent(subConfig.URL, userAgent)
	} else if subConfig.Path != "" {
		// 从本地路径读取订阅数据
		data, err = nm.fetcher.FetchSubscriptionFromPath(subConfig.Path)
	} else {
		return nil, fmt.Errorf("订阅 %s 既没有配置URL也没有配置Path", subConfig.Name)
	}

	if err != nil {
		return nil, fmt.Errorf("获取订阅失败 %s: %v", subConfig.Name, err)
	}

	// 获取对应的处理器
	processor, ok := nm.processors[strings.ToLower(subConfig.Type)]
	if !ok {
		return nil, fmt.Errorf("不支持的订阅类型: %s", subConfig.Type)
	}

	// 处理订阅数据
	nodes, err := processor.Process(data)
	if err != nil {
		return nil, fmt.Errorf("处理订阅失败 %s: %v", subConfig.Name, err)
	}

	// 添加订阅前缀
	prefixedNodes := subscription.AddSubscriptionPrefix(nodes, subConfig.Name)

	return prefixedNodes, nil
}

// writeRelayNodesToConfig 将处理后存在缓存中的 relay 节点写入配置
// 1. 根据 include_relay 确定哪些节点作为真实节点写入配置文件
// 2. 根据 relay_nodes 确定哪些节点的 tag 写入到 selector 中（不影响真实节点写入）
func (nm *NodeManager) writeRelayNodesToConfig() error {
	logger.Debug("开始将 relay 节点写入配置...")

	// 检查是否有 include_relay 配置
	if len(nm.config.Nodes.IncludeRelay) == 0 {
		logger.Debug("未配置 include_relay，跳过 relay 节点写入")
		return nil
	}

	// 遍历所有目标配置
	for _, target := range nm.config.Nodes.Targets {
		if err := nm.writeRelayNodesToTarget(target); err != nil {
			logger.Error("为目标 %s 写入 relay 节点失败: %v", target.Path, err)
			return err
		}
	}

	logger.Debug("relay 节点写入配置完成")
	return nil
}

// writeRelayNodesToTarget 为单个目标写入 relay 节点
func (nm *NodeManager) writeRelayNodesToTarget(target config.Target) error {
	// 1. 根据 include_relay 和 target.Subscriptions 筛选需要写入的 relay 节点（真实节点）
	relayNodesToWrite := nm.filterRelayNodesByIncludeAndSubscriptions(target.Subscriptions)
	if len(relayNodesToWrite) == 0 {
		logger.Debug("目标 %s: 没有符合 include_relay 和 subscriptions 条件的 relay 节点", target.Path)
		return nil
	}

	// 2. 获取目标路径下的所有配置文件
	scanner := nm.scanners[target.Path]
	configFiles, err := scanner.ScanConfigFiles()
	if err != nil {
		return fmt.Errorf("扫描配置文件失败 %s: %v", target.Path, err)
	}

	// 3. 处理每个 proxy 配置
	for _, proxy := range target.Proxies {
		// 为每个配置文件写入节点
		for _, configFile := range configFiles {
			// 写入所有符合 include_relay 条件的真实节点到配置文件
			if err := nm.writeNodesToConfigFile(configFile, proxy.InsertMarker, relayNodesToWrite); err != nil {
				return fmt.Errorf("写入节点到配置文件失败 %s: %v", configFile, err)
			}

			// 如果配置了 relay_nodes，则更新 selector 的 outbounds 列表
			if len(proxy.RelayNodes) > 0 {
				if err := nm.updateSelectorForRelayNodes(configFile, proxy.InsertMarker, relayNodesToWrite, proxy.RelayNodes); err != nil {
					return fmt.Errorf("更新 selector 失败 %s: %v", configFile, err)
				}
				logger.Debug("配置文件 %s, 选择器 %s: 成功写入 %d 个 relay 节点，并根据 relay_nodes 更新 selector", configFile, proxy.InsertMarker, len(relayNodesToWrite))
			} else {
				logger.Debug("配置文件 %s, 选择器 %s: 成功写入 %d 个 relay 节点，未配置 relay_nodes", configFile, proxy.InsertMarker, len(relayNodesToWrite))
			}
		}
	}

	return nil
}

// (removed) filterRelayNodesByInclude: previously a thin wrapper around
// filterRelayNodesByIncludeAndSubscriptions(nil). Use the latter directly.

// filterRelayNodesByIncludeAndSubscriptions 根据 include_relay 和 subscriptions 配置筛选 relay 节点
func (nm *NodeManager) filterRelayNodesByIncludeAndSubscriptions(targetSubscriptions []string) []subscription.Node {
	var result []subscription.Node

	// 创建订阅名称映射，用于快速查找
	var targetSubscriptionsMap map[string]bool
	if len(targetSubscriptions) > 0 {
		targetSubscriptionsMap = make(map[string]bool)
		for _, name := range targetSubscriptions {
			targetSubscriptionsMap[name] = true
		}
	}

	// 遍历所有缓存的 relay 展开节点
	for _, nodes := range nm.cache.relayExpanded {
		for _, node := range nodes {
			// 获取节点的 tag
			tag, ok := node["tag"].(string)
			if !ok || tag == "" {
				continue
			}

			// 1. 首先检查节点是否来自指定的订阅（如果有指定的话）
			if targetSubscriptionsMap != nil {
				isFromTargetSubscription := false
				for subName := range targetSubscriptionsMap {
					if strings.Contains(tag, fmt.Sprintf("[%s]", subName)) {
						isFromTargetSubscription = true
						break
					}
				}
				if !isFromTargetSubscription {
					continue
				}
			}

			// 2. 然后根据 include_relay 规则检查是否匹配
			shouldInclude := false
			for _, rule := range nm.config.Nodes.IncludeRelay {
				if rule.Tag == "" || len(rule.Upstream) == 0 {
					continue
				}
				if !strings.Contains(tag, rule.Tag) {
					continue
				}
				for _, up := range rule.Upstream {
					if up == "" {
						continue
					}
					if strings.Contains(tag, up) {
						shouldInclude = true
						break
					}
				}
				if shouldInclude {
					break
				}
			}

			if shouldInclude {
				result = append(result, node)
			}
		}
	}

	if len(targetSubscriptions) > 0 {
		logger.Debug("根据 subscriptions %v 和 include_relay 筛选出 %d 个节点", targetSubscriptions, len(result))
	} else {
		logger.Debug("根据 include_relay 筛选出 %d 个节点", len(result))
	}
	return result
}

// writeNodesToConfigFile 将节点写入指定的配置文件
func (nm *NodeManager) writeNodesToConfigFile(configPath, insertMarker string, nodes []subscription.Node) error {
	// 转换节点格式为 map[string]any
	nodesMaps := make([]map[string]any, len(nodes))
	for i, node := range nodes {
		nodesMaps[i] = map[string]any(node)
	}

	// 创建 updater 来更新配置文件
	updater := fileops.NewUpdater(insertMarker)

	// 获取所有 relay 订阅名称
	var relaySubNames []string
	for _, sub := range nm.config.Nodes.Subscriptions {
		if sub.Enable && strings.ToLower(sub.Type) == "relay" {
			relaySubNames = append(relaySubNames, sub.Name)
		}
	}

	// 调用 updater 的方法来插入节点
	return updater.InsertRealNodes(configPath, nodesMaps, relaySubNames)
}

// updateSelectorForRelayNodes 根据 relay_nodes 配置更新 selector 的 outbounds 列表
// 注意：这个方法只影响 selector 中的 tag 列表，不影响真实节点的写入
func (nm *NodeManager) updateSelectorForRelayNodes(configPath, insertMarker string, nodes []subscription.Node, relayNodes []string) error {
	// 转换节点格式为 map[string]any
	nodesMaps := make([]map[string]any, len(nodes))
	for i, node := range nodes {
		nodesMaps[i] = map[string]any(node)
	}

	// 创建一个临时的 updater 来更新 selector
	updater := fileops.NewUpdater(insertMarker)

	// 获取所有 relay 订阅名称
	var relaySubNames []string
	for _, sub := range nm.config.Nodes.Subscriptions {
		if sub.Enable && strings.ToLower(sub.Type) == "relay" {
			relaySubNames = append(relaySubNames, sub.Name)
		}
	}

	// 使用 relay_nodes 作为 include 关键词来过滤要添加到 selector 的节点 tag
	// 这里传入所有真实节点，但 UpdateSelectorOnly 会根据 relay_nodes 筛选要添加到 selector 的 tag
	return updater.UpdateSelectorOnly(configPath, nodesMaps, relaySubNames, relayNodes, nil)
}
