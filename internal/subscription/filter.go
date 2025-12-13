package subscription

import (
	"fmt"
	"node-box/internal/logger"
	"regexp"
	"strings"
)

var emojiRegex = regexp.MustCompile(`[\x{1F600}-\x{1F64F}\x{1F300}-\x{1F5FF}\x{1F680}-\x{1F6FF}\x{1F700}-\x{1F77F}\x{1F780}-\x{1F7FF}\x{1F800}-\x{1F8FF}\x{1F900}-\x{1F9FF}\x{1FA00}-\x{1FA6F}\x{1FA70}-\x{1FAFF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}\x{2300}-\x{23FF}]`)

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
			if strings.Contains(tag, keyword) {
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
		logger.Debug("排除节点: %d 个", excludedCount)
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

// RemoveEmoji removes emojis from node tags.
func RemoveEmoji(nodes []Node) []Node {
	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			// Replace emojis with empty string
			newTag := emojiRegex.ReplaceAllString(tag, "")
			// Trim extra spaces that might have been left behind
			node["tag"] = strings.TrimSpace(newTag)
		}
	}
	return nodes
}
