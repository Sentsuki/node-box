// Package manager provides the core node management functionality
// that coordinates all other components to manage subscription nodes.
package manager

import (
	"errors"
	"fmt"

	"node-box/internal/client"
	"node-box/internal/config"
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

// SubscriptionCache holds cached subscription data.
type SubscriptionCache struct {
	nodes         map[string][]subscription.Node // 订阅名称 -> 节点列表
	valid         bool                           // 缓存是否有效
	relayExpanded map[string][]subscription.Node // relay 展开后的节点，key 为 "relay:<subName>"
}

// NodeManager coordinates all components to implement core business logic.
// It manages the complete workflow of fetching subscriptions, processing nodes,
// and updating configuration files with caching support.
type NodeManager struct {
	config        *config.Config
	fetcher       *client.Fetcher
	processors    map[string]subscription.Processor
	filter        *subscription.Filter
	moduleManager *modules.ModuleManager
	configUpdater *modules.ConfigUpdater
	cache         *SubscriptionCache
}

// NewNodeManager creates a new NodeManager instance with all necessary components.
// It initializes HTTP client, subscription processors, file operations, and node filtering
// based on the provided configuration.
func NewNodeManager(cfg *config.Config) (*NodeManager, error) {
	httpClient, err := client.NewHTTPClient(cfg.Proxy, cfg.UserAgent)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHTTPClientCreation, err)
	}

	fetcher := client.NewFetcher(httpClient)

	processors := make(map[string]subscription.Processor)
	processors["clash"] = subscription.NewClashProcessor()
	processors["singbox"] = subscription.NewSingBoxProcessor()
	processors["relay"] = subscription.NewSingBoxProcessor()
	processors["xray"] = subscription.NewXrayProcessor()
	processors["v2ray"] = subscription.NewXrayProcessor()

	filter := subscription.NewFilter(cfg.Nodes.ExcludeKeywords)
	moduleManager := modules.NewModuleManager(cfg, fetcher)
	configUpdater := modules.NewConfigUpdater(moduleManager)

	return &NodeManager{
		config:        cfg,
		fetcher:       fetcher,
		processors:    processors,
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
func (nm *NodeManager) ClearAllCaches() {
	nm.ClearCache()
	nm.moduleManager.ClearCache()
	logger.Debug("所有缓存已清除")
}

// Cleanup performs complete cleanup of the NodeManager, releasing all resources.
func (nm *NodeManager) Cleanup() {
	if nm == nil {
		return
	}

	logger.Debug("开始清理 NodeManager 资源...")
	nm.ClearAllCaches()

	nm.config = nil
	nm.fetcher = nil
	nm.processors = nil
	nm.filter = nil
	nm.moduleManager = nil
	nm.configUpdater = nil
	nm.cache = nil

	logger.Debug("NodeManager 资源清理完成")
}

// UpdateAllConfigurations updates all configurations in sequence.
// Execution order:
//  1. 失效缓存
//  2. 获取所有订阅节点
//  3. Relay 订阅后处理（展开节点池）
//  4. 更新出站模块配置（插入真实节点）
//  5. 将 Relay 节点写入出站模块配置
//  6. 模块组装生成最终目标配置文件
func (nm *NodeManager) UpdateAllConfigurations() error {
	logger.Debug("开始更新所有配置...")

	var errs []string

	nm.InvalidateCache()
	nm.moduleManager.InvalidateCache()

	logger.Info("步骤 1/5: 获取所有订阅节点...")
	if err := nm.FetchAndCacheAllSubscriptions(); err != nil {
		logger.Warn("获取订阅数据时出现问题: %v，但继续处理", err)
	} else {
		logger.Debug("订阅节点获取成功")
	}

	logger.Info("步骤 2/5: 处理 Relay 订阅（生成节点池）...")
	if err := nm.updateRelayDetourForAllTargets(); err != nil {
		errs = append(errs, fmt.Sprintf("Relay 订阅后处理失败: %v", err))
		logger.Error("%s", errs[len(errs)-1])
	} else {
		logger.Debug("Relay 订阅后处理完成")
	}

	logger.Info("步骤 3/5: 更新出站模块配置 (插入真实节点)...")
	if err := nm.UpdateOutboundsConfigs(); err != nil {
		errs = append(errs, fmt.Sprintf("节点配置更新失败: %v", err))
		logger.Error("%s", errs[len(errs)-1])
	} else {
		logger.Debug("节点配置更新成功")
	}

	logger.Info("步骤 4/5: 将 Relay 节点写入出站模块配置...")
	if err := nm.writeRelayNodesToOutbounds(); err != nil {
		errs = append(errs, fmt.Sprintf("Relay 节点写入配置失败: %v", err))
		logger.Error("%s", errs[len(errs)-1])
	} else {
		logger.Info("Relay 节点配置完成")
	}

	logger.Info("步骤 5/5: 更新最终模块组装配置...")
	if err := nm.UpdateModuleConfigs(); err != nil {
		errs = append(errs, fmt.Sprintf("模块配置更新失败: %v", err))
		logger.Error("%s", errs[len(errs)-1])
	} else {
		logger.Debug("模块配置更新成功")
	}

	var finalErr error
	if len(errs) > 0 {
		logger.Warn("配置更新完成，但有 %d 个错误", len(errs))
		for _, msg := range errs {
			logger.Debug("  - %s", msg)
		}
		finalErr = fmt.Errorf("配置更新完成，但有 %d 个错误", len(errs))
	} else {
		logger.Debug("所有配置更新成功")
	}

	logger.Info("流程完成，清除所有缓存...")
	nm.ClearAllCaches()
	logger.Info("*****所有流程完成，缓存已清除*****")

	return finalErr
}
