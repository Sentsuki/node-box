package subscription

import (
	"fmt"
	"log"
	"strings"
)

// Filter provides node filtering functionality based on exclude keywords.
// It can filter out nodes whose tags contain specified keywords.
type Filter struct {
	excludeKeywords []string
}

// NewFilter creates a new node filter with the specified exclude keywords.
// The filter will remove nodes whose tags contain any of the provided keywords.
func NewFilter(excludeKeywords []string) *Filter {
	return &Filter{
		excludeKeywords: excludeKeywords,
	}
}

// FilterNodes filters out nodes that contain exclude keywords in their tags.
// It performs case-insensitive matching and logs excluded nodes for debugging.
func (f *Filter) FilterNodes(nodes []Node) []Node {
	var filteredNodes []Node
	excludedCount := 0

	for _, node := range nodes {
		tag, ok := node["tag"].(string)
		if !ok {
			continue
		}

		shouldExclude := false
		for _, keyword := range f.excludeKeywords {
			if strings.Contains(strings.ToLower(tag), strings.ToLower(keyword)) {
				shouldExclude = true
				excludedCount++
				break
			}
		}
		if !shouldExclude {
			filteredNodes = append(filteredNodes, node)
		}
	}

	if excludedCount > 0 {
		log.Printf("排除节点: %d 个", excludedCount)
	}

	return filteredNodes
}

// AddSubscriptionPrefix adds subscription name prefix to node tags.
// It modifies the tag field of each node to include the subscription name
// in the format "[subscription_name] original_tag".
func AddSubscriptionPrefix(nodes []Node, subName string) []Node {
	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			node["tag"] = fmt.Sprintf("[%s] %s", subName, tag)
		}
	}
	return nodes
}
