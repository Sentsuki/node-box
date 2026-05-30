package manager

import (
	"fmt"
	"strings"

	"node-box/internal/logger"
	"node-box/internal/subscription"
)

// FetchAndCacheAllSubscriptions fetches all enabled subscriptions and caches the results.
// Nodes are cached without global filtering; filtering is applied at usage time.
func (nm *NodeManager) FetchAndCacheAllSubscriptions() error {
	logger.Info("获取所有订阅节点...")

	nm.cache.nodes = make(map[string][]subscription.Node)
	nm.cache.valid = false

	var fetchErrors []string
	successCount := 0

	for _, sub := range nm.config.Nodes.Subscriptions {
		if !sub.Enable {
			continue
		}

		userAgent := sub.UserAgent
		if userAgent == "" {
			userAgent = nm.config.UserAgent
		}

		logger.Debug("获取订阅: %s", sub.Name)

		data, err := nm.fetchSubscriptionData(sub.URL, sub.Path, sub.Name, userAgent)
		if err != nil {
			errMsg := fmt.Sprintf("获取订阅失败 %s: %v", sub.Name, err)
			logger.Error("%s", errMsg)
			fetchErrors = append(fetchErrors, errMsg)
			continue
		}

		processor, ok := nm.processors[strings.ToLower(sub.Type)]
		if !ok {
			errMsg := fmt.Sprintf("%v: %s (subscription: %s)", ErrUnsupportedSubType, sub.Type, sub.Name)
			logger.Error("%s", errMsg)
			fetchErrors = append(fetchErrors, errMsg)
			continue
		}

		nodes, err := processor.Process(data)
		if err != nil {
			errMsg := fmt.Sprintf("处理订阅失败 %s: %v", sub.Name, err)
			logger.Error("%s", errMsg)
			fetchErrors = append(fetchErrors, errMsg)
			continue
		}

		// 关键词移除在 emoji 处理之前，确保关键词不受 emoji 影响
		if len(sub.RemoveKeywords) > 0 {
			nodes = subscription.RemoveKeywords(nodes, sub.RemoveKeywords)
		}

		// nil: 保留原始格式  true: 自动适配 emoji  false: 移除 emoji
		if sub.Emoji != nil {
			if *sub.Emoji {
				nodes = subscription.AutoEmoji(nodes)
			} else {
				nodes = subscription.RemoveEmoji(nodes)
			}
		}

		prefixedNodes := subscription.AddSubscriptionPrefix(nodes, sub.Name)
		nm.cache.nodes[sub.Name] = prefixedNodes
		successCount++

		if strings.ToLower(sub.Type) == "relay" {
			logger.Debug("缓存relay订阅 %s: %d 个模板节点", sub.Name, len(prefixedNodes))
		} else {
			logger.Debug("缓存订阅 %s: %d 个节点", sub.Name, len(prefixedNodes))
		}
	}

	if successCount > 0 {
		nm.cache.valid = true
	}

	if len(fetchErrors) > 0 {
		logger.Warn("订阅获取完成: 成功 %d 个，失败 %d 个", successCount, len(fetchErrors))
		for _, msg := range fetchErrors {
			logger.Debug("  - %s", msg)
		}
		if successCount == 0 {
			logger.Warn("所有订阅获取失败，但继续处理")
		} else {
			logger.Info("部分订阅获取失败，但继续处理成功的 %d 个订阅", successCount)
		}
		return nil
	}

	logger.Info("订阅缓存完成: %d 个订阅", successCount)
	return nil
}

// fetchSubscriptionData retrieves raw subscription bytes from a URL or local path.
func (nm *NodeManager) fetchSubscriptionData(url, path, name, userAgent string) ([]byte, error) {
	switch {
	case url != "":
		return nm.fetcher.FetchSubscriptionWithUserAgent(url, userAgent)
	case path != "":
		return nm.fetcher.FetchSubscriptionFromPath(path)
	default:
		return nil, fmt.Errorf("订阅 %s 既没有配置URL也没有配置Path", name)
	}
}

// FetchAllNodes retrieves nodes from all enabled subscriptions using cache.
func (nm *NodeManager) FetchAllNodes() ([]subscription.Node, error) {
	return nm.FetchNodesFromSubscriptions(nil)
}

// FetchNodesFromSubscriptions retrieves nodes from the specified subscriptions using cache.
// If subscriptionNames is nil or empty, nodes from all cached subscriptions are returned.
// Returns raw nodes without global filtering.
func (nm *NodeManager) FetchNodesFromSubscriptions(subscriptionNames []string) ([]subscription.Node, error) {
	if !nm.cache.valid {
		if err := nm.FetchAndCacheAllSubscriptions(); err != nil {
			logger.Warn("获取订阅时出现问题: %v，但继续处理", err)
		}
	}

	var targetSet map[string]bool
	if len(subscriptionNames) > 0 {
		targetSet = make(map[string]bool, len(subscriptionNames))
		for _, name := range subscriptionNames {
			targetSet[name] = true
		}
	}

	var allNodes []subscription.Node
	for _, sub := range nm.config.Nodes.Subscriptions {
		if !sub.Enable {
			continue
		}
		if targetSet != nil && !targetSet[sub.Name] {
			continue
		}
		if cachedNodes, exists := nm.cache.nodes[sub.Name]; exists {
			allNodes = append(allNodes, cachedNodes...)
		} else {
			logger.Warn("订阅 %s 不在缓存中", sub.Name)
		}
	}

	return allNodes, nil
}
