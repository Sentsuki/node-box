// Package manager provides the core node management functionality
// that coordinates all other components to manage subscription nodes.
package manager

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"node-box/internal/client"
	"node-box/internal/config"
	"node-box/internal/fileops"
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
	log.Println("订阅缓存已失效")
}

// FetchAndCacheAllSubscriptions fetches all enabled subscriptions and caches the results.
// This method should be called once per update cycle to populate the cache.
// 缓存原始节点（未经全局过滤），过滤将在使用时进行
func (nm *NodeManager) FetchAndCacheAllSubscriptions() error {
	log.Println("获取所有订阅节点...")

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

		log.Printf("获取订阅: %s", sub.Name)

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
			log.Printf("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}

		// 获取对应的处理器
		processor, ok := nm.processors[strings.ToLower(sub.Type)]
		if !ok {
			errorMsg := fmt.Sprintf("%v: %s (subscription: %s)", ErrUnsupportedSubType, sub.Type, sub.Name)
			log.Printf("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}

		// 处理订阅数据
		nodes, err := processor.Process(data)
		if err != nil {
			errorMsg := fmt.Sprintf("处理订阅失败 %s: %v", sub.Name, err)
			log.Printf("%s", errorMsg)
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
			log.Printf("relay订阅 %s 的 %d 个节点仅作为模板使用，不缓存", sub.Name, len(prefixedNodes))
		}
		successCount++
		log.Printf("缓存订阅 %s: %d 个节点", sub.Name, len(prefixedNodes))
	}

	// 标记缓存为有效（即使有部分失败）
	if successCount > 0 {
		nm.cache.valid = true
	}

	if len(fetchErrors) > 0 {
		log.Printf("订阅获取完成: 成功 %d 个，失败 %d 个", successCount, len(fetchErrors))
		log.Println("获取失败的订阅:")
		for _, errMsg := range fetchErrors {
			log.Printf("  - %s", errMsg)
		}

		if successCount == 0 {
			log.Println("警告: 所有订阅获取失败，但继续处理")
			return nil // 不返回错误，允许继续处理
		}

		log.Printf("部分订阅获取失败，但继续处理成功的 %d 个订阅", successCount)
		return nil // 不返回错误，允许继续处理
	}

	// 写出缓存文件便于检查
	if err := nm.writeCacheFiles(); err != nil {
		log.Printf("写出缓存文件失败: %v", err)
	}

	log.Printf("订阅缓存完成: %d 个订阅", successCount)
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
			log.Printf("获取订阅时出现问题: %v，但继续处理", err)
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
			log.Printf("警告: 订阅 %s 不在缓存中", sub.Name)
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
	log.Println("开始更新配置文件...")

	// 1. 获取所有节点
	if !nm.cache.valid {
		if err := nm.FetchAndCacheAllSubscriptions(); err != nil {
			log.Printf("获取订阅数据时出现问题: %v，但继续处理", err)
		}
	}

	var updateErrors []string
	totalSuccessCount := 0
	totalFileCount := 0

	for _, target := range nm.config.Nodes.Targets {
		log.Printf("处理路径: %s", target.Path)

		// 扫描配置文件
		scanner := nm.scanners[target.Path]
		configFiles, err := scanner.ScanConfigFiles()
		if err != nil {
			errorMsg := fmt.Sprintf("扫描配置文件失败 %s: %v", target.Path, err)
			log.Printf("%s", errorMsg)
			updateErrors = append(updateErrors, errorMsg)
			continue
		}

		if len(configFiles) == 0 {
			log.Printf("路径 %s 下未找到配置文件", target.Path)
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
			log.Printf("%s", errorMsg)
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
			log.Printf("路径 %s 未获取到非relay节点，跳过", target.Path)
			continue
		}

		// 2. 根据全局exclude_keywords排除节点
		filteredNodes := nm.filter.FilterNodes(nonRelayNodes)
		log.Printf("节点过滤: %d -> %d (排除 %d)", len(nonRelayNodes), len(filteredNodes), len(nonRelayNodes)-len(filteredNodes))

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
					log.Printf("%s", errorMsg)
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

		log.Printf("路径 %s 处理完成: %d 个文件", target.Path, len(configFiles))
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

	log.Printf("配置更新完成: %d 个文件", totalFileCount)
	return nil
}

// UpdateModuleConfigs updates configuration files with module data.
// It fetches all configured modules and applies them to the specified configuration files.
// Uses cached module data if available.
func (nm *NodeManager) UpdateModuleConfigs() error {
	if nm.config.Modules == nil || len(nm.config.Configs) == 0 {
		log.Println("没有配置模块或配置文件，跳过模块配置更新")
		return nil
	}

	log.Println("开始更新模块配置...")

	// 1. 获取所有模块（使用缓存机制，只在需要时请求）
	if err := nm.moduleManager.FetchAllModules(); err != nil {
		log.Printf("获取模块时出现问题: %v，但继续处理", err)
	}

	// 2. 更新每个配置文件
	var updateErrors []string
	successCount := 0

	for _, configFile := range nm.config.Configs {
		log.Printf("更新配置文件: %s (%s)", configFile.Name, configFile.Path)

		if err := nm.configUpdater.UpdateConfigFile(configFile); err != nil {
			errorMsg := fmt.Sprintf("更新配置文件失败 %s: %v", configFile.Name, err)
			log.Printf("%s", errorMsg)
			updateErrors = append(updateErrors, errorMsg)
			continue
		}

		successCount++
		log.Printf("成功更新配置文件: %s", configFile.Name)
	}

	// 3. 汇总结果
	log.Printf("模块配置更新完成: 成功 %d 个，失败 %d 个", successCount, len(updateErrors))

	if len(updateErrors) > 0 {
		log.Println("更新失败的配置文件:")
		for _, errMsg := range updateErrors {
			log.Printf("  - %s", errMsg)
		}

		if successCount > 0 {
			return fmt.Errorf("部分模块配置更新失败: %d 成功, %d 失败", successCount, len(updateErrors))
		}

		return fmt.Errorf("所有模块配置更新失败: %v", updateErrors)
	}

	if successCount == 0 {
		return fmt.Errorf("没有配置文件需要更新")
	}

	log.Println("所有模块配置更新成功")
	return nil
}

// UpdateAllConfigurations updates all configurations in sequence.
// Execution order: 1. 失效缓存 2. 节点配置 3. relay 订阅后处理 4. 模块配置
func (nm *NodeManager) UpdateAllConfigurations() error {
	log.Println("开始更新所有配置...")

	var errors []string

	// 1. 失效缓存，确保获取最新数据
	nm.InvalidateCache()
	nm.moduleManager.InvalidateCache()

	// 2. 更新节点配置
	log.Println("步骤 1/3: 更新节点配置...")
	if err := nm.UpdateAllConfigs(); err != nil {
		errorMsg := fmt.Sprintf("节点配置更新失败: %v", err)
		log.Printf("%s", errorMsg)
		errors = append(errors, errorMsg)
	} else {
		log.Println("节点配置更新成功")
	}

	// 3. relay 订阅后处理（为节点添加 detour）
	log.Println("步骤 2/3: 处理 relay 订阅 detour...")
	if err := nm.updateRelayDetourForAllTargets(); err != nil {
		errorMsg := fmt.Sprintf("relay 订阅后处理失败: %v", err)
		log.Printf("%s", errorMsg)
		errors = append(errors, errorMsg)
	} else {
		log.Println("relay 订阅后处理完成")
	}

	// 4. 更新模块配置
	log.Println("步骤 3/3: 更新模块配置...")
	if err := nm.UpdateModuleConfigs(); err != nil {
		errorMsg := fmt.Sprintf("模块配置更新失败: %v", err)
		log.Printf("%s", errorMsg)
		errors = append(errors, errorMsg)
	} else {
		log.Println("模块配置更新成功")
	}

	// 5. 汇总结果
	if len(errors) > 0 {
		log.Println("配置更新完成，但有错误:")
		for _, errMsg := range errors {
			log.Printf("  - %s", errMsg)
		}
		return fmt.Errorf("配置更新完成，但有 %d 个错误", len(errors))
	}

	log.Println("所有配置更新成功")
	return nil
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
		log.Println("未找到可用的detour标签，跳过relay处理")
		return nil
	}

	log.Printf("找到 %d 个可用的detour标签", len(detourTags))

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
			log.Printf("获取relay订阅 %s 的节点失败: %v", relaySub, err)
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
			for _, detour := range detourTags {
				if detour == "" {
					continue
				}
				nm2 := cloneMap(base)
				nm2["detour"] = detour
				if baseTag != "" {
					nm2["tag"] = fmt.Sprintf("%s -> %s", baseTag, detour)
				}
				expanded = append(expanded, subscription.Node(nm2))
			}
		}

		nm.cache.relayExpanded["relay:"+relaySub] = expanded
	}

	// 写出缓存文件
	if err := nm.writeCacheFiles(); err != nil {
		return err
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

// writeCacheFiles 将缓存写入根目录 JSON 文件，便于人工检查。
// - cache_nodes.json: 原始订阅缓存（按订阅名分组）
// - cache_relay_expanded.json: relay 展开后的节点（按配置文件路径分组）
func (nm *NodeManager) writeCacheFiles() error {
	// 写 cache_nodes.json
	nodesOut := make(map[string][]map[string]any)
	for subName, list := range nm.cache.nodes {
		var arr []map[string]any
		for _, n := range list {
			arr = append(arr, map[string]any(n))
		}
		nodesOut[subName] = arr
	}
	b1, err := json.MarshalIndent(nodesOut, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 cache_nodes 失败: %v", err)
	}
	if err := os.WriteFile("cache_nodes.json", b1, 0644); err != nil {
		return fmt.Errorf("写入 cache_nodes.json 失败: %v", err)
	}

	// 写 cache_relay_expanded.json
	relayOut := make(map[string][]map[string]any)
	for cfgPath, list := range nm.cache.relayExpanded {
		arr := make([]map[string]any, 0, len(list))
		for _, n := range list {
			arr = append(arr, map[string]any(n))
		}
		relayOut[cfgPath] = arr
	}
	b2, err := json.MarshalIndent(relayOut, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 cache_relay_expanded 失败: %v", err)
	}
	if err := os.WriteFile("cache_relay_expanded.json", b2, 0644); err != nil {
		return fmt.Errorf("写入 cache_relay_expanded.json 失败: %v", err)
	}

	return nil
}
