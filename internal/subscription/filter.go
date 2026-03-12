package subscription

import (
	"fmt"
	"node-box/internal/logger"
	"regexp"
	"strings"
	"unicode"
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
			// Replace emojis (Symbol, Other) with empty string using unicode package
			newTag := strings.Map(func(r rune) rune {
				if unicode.Is(unicode.So, r) {
					return -1
				}
				return r
			}, tag)
			// Trim extra spaces that might have been left behind
			node["tag"] = strings.TrimSpace(newTag)
		}
	}
	return nodes
}

// RemoveKeywords removes specified keywords from node tags.
// Supports glob-style wildcards: * matches any characters, ? matches a single character.
// For example:
//   - "(112人)" exact match removes "(112人)"
//   - "(*人)" removes any string matching the pattern like "(112人)", "(50人)"
//   - "节点?" removes "节点1", "节点A", etc.
func RemoveKeywords(nodes []Node, keywords []string) []Node {
	if len(keywords) == 0 {
		return nodes
	}

	// Pre-compile patterns: separate plain strings from glob patterns
	type keywordMatcher struct {
		plain string         // non-empty if this is a plain string match
		re    *regexp.Regexp // non-nil if this is a glob pattern match
	}

	var matchers []keywordMatcher
	for _, kw := range keywords {
		if strings.ContainsAny(kw, "*?") {
			// Convert glob pattern to regex
			if re, err := globToRegex(kw); err == nil {
				matchers = append(matchers, keywordMatcher{re: re})
			} else {
				logger.Warn("无效的 remove_keywords 通配符模式 '%s': %v，将作为纯文本处理", kw, err)
				matchers = append(matchers, keywordMatcher{plain: kw})
			}
		} else {
			matchers = append(matchers, keywordMatcher{plain: kw})
		}
	}

	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			newTag := tag
			for _, m := range matchers {
				if m.re != nil {
					newTag = m.re.ReplaceAllString(newTag, "")
				} else {
					newTag = strings.ReplaceAll(newTag, m.plain, "")
				}
			}
			// Clean up multiple spaces and trim
			for strings.Contains(newTag, "  ") {
				newTag = strings.ReplaceAll(newTag, "  ", " ")
			}
			node["tag"] = strings.TrimSpace(newTag)
		}
	}
	return nodes
}

// globToRegex converts a glob pattern with * and ? wildcards to a compiled regexp.
// * matches zero or more characters, ? matches exactly one character.
// All other regex special characters are escaped.
func globToRegex(pattern string) (*regexp.Regexp, error) {
	var regexStr strings.Builder
	for _, ch := range pattern {
		switch ch {
		case '*':
			regexStr.WriteString(".*")
		case '?':
			regexStr.WriteString(".")
		default:
			regexStr.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	return regexp.Compile(regexStr.String())
}
