// Package manager provides the core node management functionality
// that coordinates all other components to manage subscription nodes.
package manager

import (
	"fmt"
	"log"
	"strings"

	"node-box/internal/client"
	"node-box/internal/config"
	"node-box/internal/fileops"
	"node-box/internal/subscription"
)

// NodeManager 节点管理器，协调各个组件实现核心业务逻辑
type NodeManager struct {
	config     *config.Config
	fetcher    *client.Fetcher
	processors map[string]subscription.Processor
	scanner    *fileops.Scanner
	updater    *fileops.Updater
	filter     *subscription.Filter
}

// NewNodeManager 创建新的NodeManager实例
// 初始化所有必要的组件并建立依赖关系
func NewNodeManager(cfg *config.Config) (*NodeManager, error) {
	// 创建HTTP客户端
	httpClient, err := client.NewHTTPClient(cfg.Proxy)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP客户端失败: %v", err)
	}

	// 创建订阅获取器
	fetcher := client.NewFetcher(httpClient)

	// 创建订阅处理器映射
	processors := make(map[string]subscription.Processor)
	processors["clash"] = subscription.NewClashProcessor()
	processors["singbox"] = subscription.NewSingBoxProcessor()

	// 创建文件操作组件
	scanner := fileops.NewScanner(cfg.ConfigDir)
	updater := fileops.NewUpdater(cfg.InsertMarker)

	// 创建节点过滤器
	filter := subscription.NewFilter(cfg.ExcludeKeywords)

	return &NodeManager{
		config:     cfg,
		fetcher:    fetcher,
		processors: processors,
		scanner:    scanner,
		updater:    updater,
		filter:     filter,
	}, nil
}

// FetchAllNodes 获取所有启用订阅的节点
// 协调订阅获取和处理过程，返回处理后的节点列表
func (nm *NodeManager) FetchAllNodes() ([]subscription.Node, error) {
	var allNodes []subscription.Node
	var enabledSubscriptions []string

	log.Println("开始获取所有订阅节点...")

	for _, sub := range nm.config.Subscriptions {
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
			log.Printf("不支持的订阅类型: %s (订阅: %s)", sub.Type, sub.Name)
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

// UpdateAllConfigs 更新所有配置文件
// 协调文件扫描、节点获取和配置更新的完整流程
func (nm *NodeManager) UpdateAllConfigs() error {
	log.Println("开始更新所有配置文件...")

	// 1. 扫描配置文件
	configFiles, err := nm.scanner.ScanConfigFiles()
	if err != nil {
		return fmt.Errorf("扫描配置文件失败: %v", err)
	}

	if len(configFiles) == 0 {
		log.Printf("在目录 %s 中未找到配置文件", nm.config.ConfigDir)
		return nil
	}

	log.Printf("找到 %d 个配置文件", len(configFiles))

	// 2. 获取所有节点
	allNodes, err := nm.FetchAllNodes()
	if err != nil {
		return fmt.Errorf("获取节点失败: %v", err)
	}

	if len(allNodes) == 0 {
		log.Println("未获取到任何节点，跳过配置更新")
		return nil
	}

	// 3. 准备订阅名称列表（用于清理旧节点）
	var subscriptionNames []string
	for _, sub := range nm.config.Subscriptions {
		if sub.Enable {
			subscriptionNames = append(subscriptionNames, sub.Name)
		}
	}

	// 4. 更新每个配置文件
	var updateErrors []string
	successCount := 0

	for _, configFile := range configFiles {
		log.Printf("更新配置文件: %s", configFile)

		// 转换节点格式为updater期望的格式
		nodesMaps := make([]map[string]any, len(allNodes))
		for i, node := range allNodes {
			nodesMaps[i] = map[string]any(node)
		}

		err := nm.updater.UpdateConfigFile(configFile, nodesMaps, subscriptionNames)
		if err != nil {
			errorMsg := fmt.Sprintf("更新配置文件失败 %s: %v", configFile, err)
			log.Printf(errorMsg)
			updateErrors = append(updateErrors, errorMsg)
			continue
		}

		successCount++
		log.Printf("成功更新配置文件: %s", configFile)
	}

	// 5. 汇总结果
	log.Printf("配置更新完成: 成功 %d 个，失败 %d 个", successCount, len(updateErrors))

	if len(updateErrors) > 0 {
		log.Println("更新失败的文件:")
		for _, errMsg := range updateErrors {
			log.Printf("  - %s", errMsg)
		}

		// 如果有部分成功，返回包含错误信息的错误，但不完全失败
		if successCount > 0 {
			return fmt.Errorf("部分配置文件更新失败: %d 个成功，%d 个失败", successCount, len(updateErrors))
		}

		// 如果全部失败，返回更严重的错误
		return fmt.Errorf("所有配置文件更新失败: %v", updateErrors)
	}

	log.Println("所有配置文件更新成功")
	return nil
}
