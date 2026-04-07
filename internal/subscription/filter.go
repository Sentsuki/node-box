package subscription

import (
	"fmt"
	"node-box/internal/logger"
	"node-box/internal/utils"
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
			if utils.ContainsIgnoreEmoji(tag, keyword) {
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

// emojiMapping defines the mapping from keywords to emoji for auto-assignment.
// Order matters: more specific patterns should come before general ones.
var emojiMapping = []struct {
	keywords []string
	emoji    string
}{
	{[]string{"阿根廷", "AR", "Argentina"}, "🇦🇷"},
	{[]string{"澳大利亚", "澳洲", "AU", "Australia"}, "🇦🇺"},
	{[]string{"奥地利", "AT", "Austria"}, "🇦🇹"},
	{[]string{"孟加拉", "BD", "Bangladesh"}, "🇧🇩"},
	{[]string{"比利时", "BE", "Belgium"}, "🇧🇪"},
	{[]string{"巴西", "BR", "Brazil"}, "🇧🇷"},
	{[]string{"加拿大", "CA", "Canada"}, "🇨🇦"},
	{[]string{"智利", "CL", "Chile"}, "🇨🇱"},
	{[]string{"哥伦比亚", "CO", "Colombia"}, "🇨🇴"},
	{[]string{"捷克", "CZ", "Czech"}, "🇨🇿"},
	{[]string{"丹麦", "DK", "Denmark"}, "🇩🇰"},
	{[]string{"埃及", "EG", "Egypt"}, "🇪🇬"},
	{[]string{"芬兰", "FI", "Finland"}, "🇫🇮"},
	{[]string{"法国", "FR", "France"}, "🇫🇷"},
	{[]string{"德国", "DE", "Germany"}, "🇩🇪"},
	{[]string{"香港", "HK", "Hong Kong", "HongKong"}, "🇭🇰"},
	{[]string{"匈牙利", "HU", "Hungary"}, "🇭🇺"},
	{[]string{"冰岛", "IS", "Iceland"}, "🇮🇸"},
	{[]string{"印度", "IN", "India"}, "🇮🇳"},
	{[]string{"印尼", "印度尼西亚", "ID", "Indonesia"}, "🇮🇩"},
	{[]string{"爱尔兰", "IE", "Ireland"}, "🇮🇪"},
	{[]string{"以色列", "IL", "Israel"}, "🇮🇱"},
	{[]string{"意大利", "IT", "Italy"}, "🇮🇹"},
	{[]string{"日本", "JP", "Japan"}, "🇯🇵"},
	{[]string{"哈萨克斯坦", "KZ", "Kazakhstan"}, "🇰🇿"},
	{[]string{"肯尼亚", "KE", "Kenya"}, "🇰🇪"},
	{[]string{"韩国", "KR", "Korea"}, "🇰🇷"},
	{[]string{"马来西亚", "MY", "大马", "Malaysia"}, "🇲🇾"},
	{[]string{"墨西哥", "MX", "Mexico"}, "🇲🇽"},
	{[]string{"荷兰", "NL", "Netherlands"}, "🇳🇱"},
	{[]string{"新西兰", "NZ", "New Zealand"}, "🇳🇿"},
	{[]string{"尼日利亚", "NG", "Nigeria"}, "🇳🇬"},
	{[]string{"挪威", "NO", "Norway"}, "🇳🇴"},
	{[]string{"巴基斯坦", "PK", "Pakistan"}, "🇵🇰"},
	{[]string{"菲律宾", "PH", "Philippines"}, "🇵🇭"},
	{[]string{"波兰", "PL", "Poland"}, "🇵🇱"},
	{[]string{"葡萄牙", "PT", "Portugal"}, "🇵🇹"},
	{[]string{"罗马尼亚", "RO", "Romania"}, "🇷🇴"},
	{[]string{"俄罗斯", "RU", "Russia"}, "🇷🇺"},
	{[]string{"沙特", "SA", "Saudi"}, "🇸🇦"},
	{[]string{"新加坡", "SG", "Singapore"}, "🇸🇬"},
	{[]string{"南非", "ZA", "South Africa"}, "🇿🇦"},
	{[]string{"西班牙", "ES", "Spain"}, "🇪🇸"},
	{[]string{"瑞典", "SE", "Sweden"}, "🇸🇪"},
	{[]string{"瑞士", "CH", "Switzerland"}, "🇨🇭"},
	{[]string{"台湾", "TW", "Taiwan"}, "🇹🇼"},
	{[]string{"泰国", "TH", "Thailand"}, "🇹🇭"},
	{[]string{"土耳其", "TR", "Turkey", "Türkiye"}, "🇹🇷"},
	{[]string{"阿联酋", "AE", "UAE", "Dubai", "迪拜"}, "🇦🇪"},
	{[]string{"乌克兰", "UA", "Ukraine"}, "🇺🇦"},
	{[]string{"英国", "UK", "GB", "United Kingdom", "Britain"}, "🇬🇧"},
	{[]string{"美国", "US", "USA", "United States", "America"}, "🇺🇸"},
	{[]string{"越南", "VN", "Vietnam"}, "🇻🇳"},
}

// matchEmoji returns the appropriate emoji for a given node tag based on keyword matching.
// Returns "🇺🇳" if no specific region is matched.
func matchEmoji(tag string) string {
	upperTag := strings.ToUpper(tag)
	for _, mapping := range emojiMapping {
		for _, keyword := range mapping.keywords {
			if strings.Contains(upperTag, strings.ToUpper(keyword)) {
				return mapping.emoji
			}
		}
	}
	return "🇺🇳"
}

// AutoEmoji removes existing emojis from node tags and adds appropriate emoji
// based on geographic/keyword matching of the node name.
// This avoids problematic emoji from subscription sources and ensures consistency.
func AutoEmoji(nodes []Node) []Node {
	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			// Step 1: Remove existing emojis
			cleanTag := strings.Map(func(r rune) rune {
				if unicode.Is(unicode.So, r) {
					return -1
				}
				return r
			}, tag)
			cleanTag = strings.TrimSpace(cleanTag)

			// Step 2: Auto-assign emoji based on node name
			emoji := matchEmoji(cleanTag)
			node["tag"] = emoji + " " + cleanTag
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
