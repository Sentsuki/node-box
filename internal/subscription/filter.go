package subscription

import (
	"fmt"
	"log"
	"strings"
)

// Filter 节点过滤器
type Filter struct {
	excludeKeywords []string
}

// NewFilter 创建新的节点过滤器
func NewFilter(excludeKeywords []string) *Filter {
	return &Filter{
		excludeKeywords: excludeKeywords,
	}
}

// FilterNodes 过滤排除关键词的节点
func (f *Filter) FilterNodes(nodes []Node) []Node {
	var filteredNodes []Node
	for _, node := range nodes {
		tag, ok := node["tag"].(string)
		if !ok {
			continue
		}

		shouldExclude := false
		for _, keyword := range f.excludeKeywords {
			if strings.Contains(strings.ToLower(tag), strings.ToLower(keyword)) {
				log.Printf("排除节点: %s (包含关键词: %s)", tag, keyword)
				shouldExclude = true
				break
			}
		}
		if !shouldExclude {
			filteredNodes = append(filteredNodes, node)
		}
	}
	return filteredNodes
}

// AddSubscriptionPrefix 为节点添加订阅前缀
func AddSubscriptionPrefix(nodes []Node, subName string) []Node {
	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			node["tag"] = fmt.Sprintf("[%s] %s", subName, tag)
		}
	}
	return nodes
}
