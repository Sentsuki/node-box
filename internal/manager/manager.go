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
}

// NodeManager coordinates all components to implement core business logic.
// It manages the complete workflow of fetching subscriptions, processing nodes,
// and updating configuration files with caching support.
type NodeManager struct {
	config        *config.Config
	fetcher       *client.Fetcher
	processors    map[string]subscription.Processor
	scanners      map[string]*fileops.Scanner
	updaters      map[string]*fileops.Updater
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

	// 为每个配置路径创建扫描器和更新器
	scanners := make(map[string]*fileops.Scanner)
	updaters := make(map[string]*fileops.Updater)

	for _, target := range cfg.Nodes.Targets {
		scanners[target.InsertPath] = fileops.NewScanner(target.InsertPath, target.IsFile)
		updaters[target.InsertPath] = fileops.NewUpdater(target.InsertMarker)
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
		updaters:      updaters,
		filter:        filter,
		moduleManager: moduleManager,
		configUpdater: configUpdater,
		cache: &SubscriptionCache{
			nodes: make(map[string][]subscription.Node),
			valid: false,
		},
	}, nil
}

// InvalidateCache invalidates the subscription cache, forcing a fresh fetch on next request.
func (nm *NodeManager) InvalidateCache() {
	nm.cache.valid = false
	nm.cache.nodes = make(map[string][]subscription.Node)
	log.Println("订阅缓存已失效")
}

// FetchAndCacheAllSubscriptions fetches all enabled subscriptions and caches the results.
// This method should be called once per update cycle to populate the cache.
func (nm *NodeManager) FetchAndCacheAllSubscriptions() error {
	log.Println("开始获取并缓存所有订阅节点...")

	// 清空缓存
	nm.cache.nodes = make(map[string][]subscription.Node)
	nm.cache.valid = false

	var fetchErrors []string
	successCount := 0

	for _, sub := range nm.config.Nodes.Subscriptions {
		if !sub.Enable {
			log.Printf("跳过已禁用的订阅: %s", sub.Name)
			continue
		}

		log.Printf("获取订阅: %s", sub.Name)

		// 获取订阅数据（带重试）
		data, err := nm.fetcher.FetchSubscription(sub.URL)
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

		// 过滤节点
		filteredNodes := nm.filter.FilterNodes(nodes)
		log.Printf("订阅 %s: 原始节点 %d 个，过滤后 %d 个", sub.Name, len(nodes), len(filteredNodes))

		// 添加订阅前缀
		prefixedNodes := subscription.AddSubscriptionPrefix(filteredNodes, sub.Name)

		// 缓存节点
		nm.cache.nodes[sub.Name] = prefixedNodes
		successCount++
		log.Printf("成功缓存订阅 %s: %d 个节点", sub.Name, len(prefixedNodes))
	}

	// 标记缓存为有效（即使有部分失败）
	if successCount > 0 {
		nm.cache.valid = true
		log.Printf("订阅缓存完成: 成功 %d 个，失败 %d 个", successCount, len(fetchErrors))
	}

	if len(fetchErrors) > 0 {
		log.Println("获取失败的订阅:")
		for _, errMsg := range fetchErrors {
			log.Printf("  - %s", errMsg)
		}

		if successCount == 0 {
			return fmt.Errorf("所有订阅获取失败: %v", fetchErrors)
		}

		return fmt.Errorf("部分订阅获取失败: %d 成功, %d 失败", successCount, len(fetchErrors))
	}

	log.Printf("所有订阅缓存成功，总计 %d 个订阅", successCount)
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
func (nm *NodeManager) FetchNodesFromSubscriptions(subscriptionNames []string) ([]subscription.Node, error) {
	// 如果缓存无效，先获取所有订阅
	if !nm.cache.valid {
		log.Println("缓存无效，重新获取所有订阅...")
		if err := nm.FetchAndCacheAllSubscriptions(); err != nil {
			return nil, fmt.Errorf("获取订阅失败: %v", err)
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
		log.Printf("从缓存获取指定订阅节点: %v", subscriptionNames)
	} else {
		log.Println("从缓存获取所有订阅节点...")
	}

	// 从缓存中获取节点
	for _, sub := range nm.config.Nodes.Subscriptions {
		if !sub.Enable {
			continue
		}

		// 如果指定了订阅名称，只处理指定的订阅
		if targetSubscriptions != nil && !targetSubscriptions[sub.Name] {
			continue
		}

		// 从缓存获取节点
		if cachedNodes, exists := nm.cache.nodes[sub.Name]; exists {
			allNodes = append(allNodes, cachedNodes...)
			log.Printf("从缓存获取订阅 %s: %d 个节点", sub.Name, len(cachedNodes))
		} else {
			log.Printf("警告: 订阅 %s 不在缓存中", sub.Name)
		}
	}

	log.Printf("总共从缓存获取到 %d 个节点", len(allNodes))
	return allNodes, nil
}

// UpdateAllConfigs updates all configuration files with new proxy nodes.
// It coordinates the complete workflow of file scanning, node fetching,
// and configuration updating with comprehensive error handling and caching.
func (nm *NodeManager) UpdateAllConfigs() error {
	log.Println("开始更新所有配置文件...")

	// 1. 确保缓存是最新的（只获取一次所有订阅）
	if !nm.cache.valid {
		log.Println("缓存无效，获取所有订阅数据...")
		if err := nm.FetchAndCacheAllSubscriptions(); err != nil {
			return fmt.Errorf("获取订阅数据失败: %v", err)
		}
	}

	// 2. 处理每个配置路径
	var updateErrors []string
	totalSuccessCount := 0
	totalFileCount := 0

	for _, target := range nm.config.Nodes.Targets {
		log.Printf("处理配置路径: %s (marker: %s)", target.InsertPath, target.InsertMarker)

		// 3. 从缓存获取当前目标的节点（根据订阅过滤）
		targetNodes, err := nm.FetchNodesFromSubscriptions(target.Subscriptions)
		if err != nil {
			errorMsg := fmt.Sprintf("获取节点失败 %s: %v", target.InsertPath, err)
			log.Printf("%s", errorMsg)
			updateErrors = append(updateErrors, errorMsg)
			continue
		}

		if len(targetNodes) == 0 {
			log.Printf("路径 %s 未获取到节点，跳过更新", target.InsertPath)
			continue
		}

		// 4. 准备订阅名称列表（用于清理旧节点）
		var subscriptionNames []string
		if len(target.Subscriptions) > 0 {
			// 使用指定的订阅
			subscriptionNames = target.Subscriptions
		} else {
			// 使用所有启用的订阅
			for _, sub := range nm.config.Nodes.Subscriptions {
				if sub.Enable {
					subscriptionNames = append(subscriptionNames, sub.Name)
				}
			}
		}

		// 5. 转换节点格式为updater期望的格式
		nodesMaps := make([]map[string]any, len(targetNodes))
		for i, node := range targetNodes {
			nodesMaps[i] = map[string]any(node)
		}

		// 6. 扫描当前路径下的配置文件
		scanner := nm.scanners[target.InsertPath]
		configFiles, err := scanner.ScanConfigFiles()
		if err != nil {
			errorMsg := fmt.Sprintf("扫描配置文件失败 %s: %v", target.InsertPath, err)
			log.Printf("%s", errorMsg)
			updateErrors = append(updateErrors, errorMsg)
			continue
		}

		if len(configFiles) == 0 {
			if target.IsFile {
				log.Printf("指定的配置文件不存在: %s", target.InsertPath)
			} else {
				log.Printf("路径 %s 下未找到配置文件", target.InsertPath)
			}
			continue
		}

		log.Printf("在路径 %s 下找到 %d 个配置文件，节点数量: %d", target.InsertPath, len(configFiles), len(targetNodes))
		totalFileCount += len(configFiles)

		// 7. 获取对应的更新器
		updater := nm.updaters[target.InsertPath]

		// 8. 更新当前路径下的每个配置文件
		pathSuccessCount := 0
		for _, configFile := range configFiles {
			log.Printf("更新配置文件: %s", configFile)

			err := updater.UpdateConfigFile(configFile, nodesMaps, subscriptionNames)
			if err != nil {
				errorMsg := fmt.Sprintf("更新配置文件失败 %s: %v", configFile, err)
				log.Printf("%s", errorMsg)
				updateErrors = append(updateErrors, errorMsg)
				continue
			}

			pathSuccessCount++
			log.Printf("成功更新配置文件: %s", configFile)
		}

		totalSuccessCount += pathSuccessCount
		log.Printf("路径 %s 处理完成: 成功 %d 个，失败 %d 个",
			target.InsertPath, pathSuccessCount, len(configFiles)-pathSuccessCount)
	}

	// 9. 汇总结果
	log.Printf("所有配置更新完成: 总文件 %d 个，成功 %d 个，失败 %d 个",
		totalFileCount, totalSuccessCount, len(updateErrors))

	if len(updateErrors) > 0 {
		log.Println("更新失败的文件:")
		for _, errMsg := range updateErrors {
			log.Printf("  - %s", errMsg)
		}

		// 如果有部分成功，返回包含错误信息的错误，但不完全失败
		if totalSuccessCount > 0 {
			return fmt.Errorf("%w: %d successful, %d failed", ErrPartialUpdateFailure, totalSuccessCount, len(updateErrors))
		}

		// 如果全部失败，返回更严重的错误
		return fmt.Errorf("%w: %v", ErrAllUpdatesFailure, updateErrors)
	}

	if totalFileCount == 0 {
		log.Printf("%v in all configured paths", ErrNoConfigFiles)
		return fmt.Errorf("%w in all configured paths", ErrNoConfigFiles)
	}

	log.Println("所有配置文件更新成功")
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
		return fmt.Errorf("获取模块失败: %v", err)
	}

	// 2. 更新每个配置文件
	var updateErrors []string
	successCount := 0

	for _, configFile := range nm.config.Configs {
		log.Printf("更新配置文件: %s (%s)", configFile.Name, configFile.File)

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

// UpdateAllConfigurations updates both node configurations and module configurations.
// It coordinates the complete workflow of updating all types of configurations.
// Execution order: 1. 失效缓存 2. 节点配置 3. 模块配置
func (nm *NodeManager) UpdateAllConfigurations() error {
	log.Println("开始更新所有配置...")

	var errors []string

	// 1. 失效缓存，确保获取最新数据
	nm.InvalidateCache()
	nm.moduleManager.InvalidateCache()

	// 2. 更新节点配置
	log.Println("步骤 1/2: 更新节点配置...")
	if err := nm.UpdateAllConfigs(); err != nil {
		errorMsg := fmt.Sprintf("节点配置更新失败: %v", err)
		log.Printf("%s", errorMsg)
		errors = append(errors, errorMsg)
	} else {
		log.Println("节点配置更新成功")
	}

	// 3. 更新模块配置
	log.Println("步骤 2/2: 更新模块配置...")
	if err := nm.UpdateModuleConfigs(); err != nil {
		errorMsg := fmt.Sprintf("模块配置更新失败: %v", err)
		log.Printf("%s", errorMsg)
		errors = append(errors, errorMsg)
	} else {
		log.Println("模块配置更新成功")
	}

	// 4. 汇总结果
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
