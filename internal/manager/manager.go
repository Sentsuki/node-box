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

// NodeManager coordinates all components to implement core business logic.
// It manages the complete workflow of fetching subscriptions, processing nodes,
// and updating configuration files.
type NodeManager struct {
	config        *config.Config
	fetcher       *client.Fetcher
	processors    map[string]subscription.Processor
	scanners      map[string]*fileops.Scanner
	updaters      map[string]*fileops.Updater
	filter        *subscription.Filter
	moduleManager *modules.ModuleManager
	configUpdater *modules.ConfigUpdater
}

// NewNodeManager creates a new NodeManager instance with all necessary components.
// It initializes HTTP client, subscription processors, file operations, and node filtering
// based on the provided configuration.
func NewNodeManager(cfg *config.Config) (*NodeManager, error) {
	// 创建HTTP客户端
	httpClient, err := client.NewHTTPClient(cfg.Proxy)
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
		scanners[target.InsertPath] = fileops.NewScanner(target.InsertPath)
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
	}, nil
}

// FetchAllNodes retrieves nodes from all enabled subscriptions.
// It coordinates the subscription fetching and processing workflow,
// returning a list of processed and filtered proxy nodes.
func (nm *NodeManager) FetchAllNodes() ([]subscription.Node, error) {
	var allNodes []subscription.Node
	var enabledSubscriptions []string

	log.Println("开始获取所有订阅节点...")

	for _, sub := range nm.config.Nodes.Subscriptions {
		if !sub.Enable {
			log.Printf("跳过已禁用的订阅: %s", sub.Name)
			continue
		}

		enabledSubscriptions = append(enabledSubscriptions, sub.Name)

		// 获取订阅数据
		data, err := nm.fetcher.FetchSubscription(sub.URL)
		if err != nil {
			log.Printf("获取订阅失败 %s: %v", sub.Name, err)
			continue
		}

		// 获取对应的处理器
		processor, ok := nm.processors[strings.ToLower(sub.Type)]
		if !ok {
			log.Printf("%v: %s (subscription: %s)", ErrUnsupportedSubType, sub.Type, sub.Name)
			continue
		}

		// 处理订阅数据
		nodes, err := processor.Process(data)
		if err != nil {
			log.Printf("处理订阅失败 %s: %v", sub.Name, err)
			continue
		}

		// 过滤节点
		filteredNodes := nm.filter.FilterNodes(nodes)
		log.Printf("订阅 %s: 原始节点 %d 个，过滤后 %d 个", sub.Name, len(nodes), len(filteredNodes))

		// 添加订阅前缀
		prefixedNodes := subscription.AddSubscriptionPrefix(filteredNodes, sub.Name)

		// 添加到总节点列表
		allNodes = append(allNodes, prefixedNodes...)
	}

	log.Printf("总共获取到 %d 个节点", len(allNodes))
	return allNodes, nil
}

// UpdateAllConfigs updates all configuration files with new proxy nodes.
// It coordinates the complete workflow of file scanning, node fetching,
// and configuration updating with comprehensive error handling.
func (nm *NodeManager) UpdateAllConfigs() error {
	log.Println("开始更新所有配置文件...")

	// 1. 获取所有节点
	allNodes, err := nm.FetchAllNodes()
	if err != nil {
		return fmt.Errorf("获取节点失败: %v", err)
	}

	if len(allNodes) == 0 {
		log.Printf("%v, skipping configuration update", ErrNoNodes)
		return fmt.Errorf("%w", ErrNoNodes)
	}

	// 2. 准备订阅名称列表（用于清理旧节点）
	var subscriptionNames []string
	for _, sub := range nm.config.Nodes.Subscriptions {
		if sub.Enable {
			subscriptionNames = append(subscriptionNames, sub.Name)
		}
	}

	// 3. 转换节点格式为updater期望的格式
	nodesMaps := make([]map[string]any, len(allNodes))
	for i, node := range allNodes {
		nodesMaps[i] = map[string]any(node)
	}

	// 4. 处理每个配置路径
	var updateErrors []string
	totalSuccessCount := 0
	totalFileCount := 0

	for _, target := range nm.config.Nodes.Targets {
		log.Printf("处理配置路径: %s (marker: %s)", target.InsertPath, target.InsertMarker)

		// 扫描当前路径下的配置文件
		scanner := nm.scanners[target.InsertPath]
		configFiles, err := scanner.ScanConfigFiles()
		if err != nil {
			errorMsg := fmt.Sprintf("扫描配置文件失败 %s: %v", target.InsertPath, err)
			log.Printf("%s", errorMsg)
			updateErrors = append(updateErrors, errorMsg)
			continue
		}

		if len(configFiles) == 0 {
			log.Printf("路径 %s 下未找到配置文件", target.InsertPath)
			continue
		}

		log.Printf("在路径 %s 下找到 %d 个配置文件", target.InsertPath, len(configFiles))
		totalFileCount += len(configFiles)

		// 获取对应的更新器
		updater := nm.updaters[target.InsertPath]

		// 更新当前路径下的每个配置文件
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

	// 5. 汇总结果
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
func (nm *NodeManager) UpdateModuleConfigs() error {
	if nm.config.Modules == nil || len(nm.config.Configs) == 0 {
		log.Println("没有配置模块或配置文件，跳过模块配置更新")
		return nil
	}

	log.Println("开始更新模块配置...")

	// 1. 获取所有模块
	if err := nm.moduleManager.FetchAllModules(); err != nil {
		return fmt.Errorf("获取模块失败: %v", err)
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

// UpdateAllConfigurations updates both node configurations and module configurations.
// It coordinates the complete workflow of updating all types of configurations.
// Execution order: 1. 节点配置 2. 模块配置 3. 更新配置
func (nm *NodeManager) UpdateAllConfigurations() error {
	log.Println("开始更新所有配置...")

	var errors []string

	// 1. 更新节点配置
	log.Println("步骤 1/2: 更新节点配置...")
	if err := nm.UpdateAllConfigs(); err != nil {
		errorMsg := fmt.Sprintf("节点配置更新失败: %v", err)
		log.Printf("%s", errorMsg)
		errors = append(errors, errorMsg)
	} else {
		log.Println("节点配置更新成功")
	}

	// 2. 更新模块配置
	log.Println("步骤 2/2: 更新模块配置...")
	if err := nm.UpdateModuleConfigs(); err != nil {
		errorMsg := fmt.Sprintf("模块配置更新失败: %v", err)
		log.Printf("%s", errorMsg)
		errors = append(errors, errorMsg)
	} else {
		log.Println("模块配置更新成功")
	}

	// 3. 汇总结果
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
