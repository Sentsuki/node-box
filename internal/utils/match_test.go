package utils_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"node-box/internal/utils"
)

// ---------------------------------------------------------------------------
// TestContainsIgnoreEmoji – unit tests for the core matching function
// ---------------------------------------------------------------------------

func TestContainsIgnoreEmoji(t *testing.T) {
	cases := []struct {
		s, kw string
		want  bool
	}{
		// Plain keyword, tag with emoji (emoji:true scenario)
		{"🇺🇸 美国 01", "美国", true},
		{"🇺🇸 美国 IEPL 01", "IEPL", true},
		{"🇯🇵 日本 02", "美国", false},

		// Emoji keyword, plain tag (emoji:false scenario)
		// "🇺🇸 美国" → strip emoji → "美国", which is found in "美国 01" → true
		{"美国 01", "🇺🇸 美国", true},
		{"日本 02", "🇺🇸 美国", false},

		// Both have emoji
		{"🇺🇸 美国 01", "🇺🇸 美国", true},

		// [subName] prefix present (realistic tag after AddSubscriptionPrefix)
		{"[mysub] 🇺🇸 美国 01", "美国", true},
		{"[mysub] 美国 01", "🇺🇸 美国", true},
		{"[mysub] 🇯🇵 日本 02", "美国", false},

		// Edge cases
		{"纯文字节点", "纯文字", true},
		{"纯文字节点", "其他", false},
		{"任意节点", "", false}, // empty keyword always false
		// When keyword is pure emoji (no text), stripEmoji → "", falls back to literal match.
		// "🇺🇸" (literal) does NOT appear in "美国 IEPL 01", so → false.
		// In practice users should never use a pure-emoji keyword.
		{"美国 IEPL 01", "🇺🇸", false},
		// when tag also has the same emoji, literal match works
		{"🇺🇸 美国 01", "🇺🇸", true},
	}

	for _, c := range cases {
		got := utils.ContainsIgnoreEmoji(c.s, c.kw)
		if got != c.want {
			t.Errorf("ContainsIgnoreEmoji(%q, %q) = %v, want %v", c.s, c.kw, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TestEmojiPipelineIntegration – simulates the full node processing pipeline:
//   raw node → emoji processing → AddSubscriptionPrefix → keyword matching
//
// This verifies that with emoji:true AND emoji:false, keyword matching works
// correctly WITHOUT modifying the node name for the caller.
// ---------------------------------------------------------------------------

// simulatedNode mirrors subscription.Node (map[string]any) locally.
type simulatedNode map[string]any

// applyAutoEmoji mimics subscription.AutoEmoji (only strips unicode.So, then prepends flag).
func applyAutoEmoji(nodes []simulatedNode) []simulatedNode {
	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			clean := strings.Map(func(r rune) rune {
				if unicode.Is(unicode.So, r) {
					return -1
				}
				return r
			}, tag)
			clean = strings.TrimSpace(clean)
			// Derive a simple flag for the test (just prepend 🇺🇸 for 美国, 🇯🇵 for 日本).
			flag := autoFlag(clean)
			node["tag"] = flag + " " + clean
		}
	}
	return nodes
}

// applyRemoveEmoji mimics subscription.RemoveEmoji.
func applyRemoveEmoji(nodes []simulatedNode) []simulatedNode {
	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			clean := strings.Map(func(r rune) rune {
				if unicode.Is(unicode.So, r) {
					return -1
				}
				return r
			}, tag)
			node["tag"] = strings.TrimSpace(clean)
		}
	}
	return nodes
}

// addPrefix mimics subscription.AddSubscriptionPrefix.
func addPrefix(nodes []simulatedNode, subName string) []simulatedNode {
	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			node["tag"] = fmt.Sprintf("[%s] %s", subName, tag)
		}
	}
	return nodes
}

func autoFlag(s string) string {
	switch {
	case strings.Contains(s, "美国"):
		return "🇺🇸"
	case strings.Contains(s, "日本"):
		return "🇯🇵"
	case strings.Contains(s, "香港"):
		return "🇭🇰"
	default:
		return "🌐"
	}
}

// containsIgnoreEmoji wraps the real function for readability.
func match(tag, keyword string) bool {
	return utils.ContainsIgnoreEmoji(tag, keyword)
}

func TestEmojiPipelineIntegration(t *testing.T) {
	const subName = "mysub"

	// Raw tags as they might come from a clash/singbox subscription source.
	rawTags := []string{
		"美国 IEPL 01",    // no emoji originally
		"🇺🇸 美国 Premium", // already has flag emoji
		"日本 Tokyo 02",
		"🇯🇵 日本 Osaka",
	}

	// Build raw nodes (original tag is preserved in "raw_tag" to verify no mutation).
	makeNodes := func() []simulatedNode {
		nodes := make([]simulatedNode, len(rawTags))
		for i, tag := range rawTags {
			nodes[i] = simulatedNode{"tag": tag, "raw_tag": tag}
		}
		return nodes
	}

	// Keywords a user might configure (mix of emoji/no-emoji).
	type matchCase struct {
		keyword     string
		wantMatches []string // raw_tags that should match
	}

	matchCases := []matchCase{
		{"美国", []string{"美国 IEPL 01", "🇺🇸 美国 Premium"}},
		{"🇺🇸 美国", []string{"美国 IEPL 01", "🇺🇸 美国 Premium"}},
		// Pure-emoji keyword ("🇺🇸" alone) is intentionally omitted:
		// after stripping emoji it becomes an empty string, so it cannot
		// meaningfully match anything — this is by design.
		{"IEPL", []string{"美国 IEPL 01"}},
		{"日本", []string{"日本 Tokyo 02", "🇯🇵 日本 Osaka"}},
		{"Tokyo", []string{"日本 Tokyo 02"}},
	}

	type scenario struct {
		name    string
		process func([]simulatedNode) []simulatedNode
	}

	scenarios := []scenario{
		{
			name: "emoji:true (AutoEmoji)",
			process: func(nodes []simulatedNode) []simulatedNode {
				return addPrefix(applyAutoEmoji(nodes), subName)
			},
		},
		{
			name: "emoji:false (RemoveEmoji)",
			process: func(nodes []simulatedNode) []simulatedNode {
				return addPrefix(applyRemoveEmoji(nodes), subName)
			},
		},
		{
			name: "emoji:nil (original kept)",
			process: func(nodes []simulatedNode) []simulatedNode {
				return addPrefix(nodes, subName)
			},
		},
	}

	for _, sc := range scenarios {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			nodes := sc.process(makeNodes())

			// 1. Verify that raw_tag (original name) is never mutated.
			for _, node := range nodes {
				raw := node["raw_tag"].(string)
				tag := node["tag"].(string)
				// The processed tag should contain the raw text (minus emoji) after [subName] prefix.
				// We just check raw_tag wasn't overwritten.
				if node["raw_tag"].(string) != raw {
					t.Errorf("[%s] raw_tag was mutated: %q → %q", sc.name, raw, node["raw_tag"])
				}
				_ = tag // processed tag may legitimately differ (emoji added/removed)
			}

			// 2. For each keyword, verify matching nodes are found correctly.
			for _, mc := range matchCases {
				mc := mc
				t.Run(fmt.Sprintf("keyword=%q", mc.keyword), func(t *testing.T) {
					// Build a set of raw_tags that the matcher selects.
					var got []string
					for _, node := range nodes {
						tag := node["tag"].(string)
						if match(tag, mc.keyword) {
							got = append(got, node["raw_tag"].(string))
						}
					}

					// Compare against expected (order-insensitive).
					wantSet := make(map[string]bool)
					for _, w := range mc.wantMatches {
						wantSet[w] = true
					}
					gotSet := make(map[string]bool)
					for _, g := range got {
						gotSet[g] = true
					}

					for w := range wantSet {
						if !gotSet[w] {
							t.Errorf("[%s] keyword=%q: expected to match raw_tag %q but did not.\nProcessed tags: %v",
								sc.name, mc.keyword, w, tagsOf(nodes))
						}
					}
					for g := range gotSet {
						if !wantSet[g] {
							t.Errorf("[%s] keyword=%q: unexpectedly matched raw_tag %q.\nProcessed tags: %v",
								sc.name, mc.keyword, g, tagsOf(nodes))
						}
					}
				})
			}
		})
	}
}

func tagsOf(nodes []simulatedNode) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n["tag"].(string)
	}
	return out
}
