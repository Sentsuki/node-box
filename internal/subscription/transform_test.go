package subscription

import (
	"strings"
	"testing"

	"node-box/internal/model"
	"node-box/internal/node"
)

// assignTagEmojiDefault applies only the built-in table, which is what a config
// without emoji_overrides gets.
func assignTagEmojiDefault(nodes []node.Node) []node.Node {
	return assignTagEmoji(nodes, newEmojiTable(nil))
}

// ---------------------------------------------------------------------------
// dropExcluded
// ---------------------------------------------------------------------------

func TestDropExcluded_ExcludesMatchingTags(t *testing.T) {
	excluded := []string{"过期", "测试"}
	nodes := []node.Node{
		{"tag": "🇺🇸 美国 01"},
		{"tag": "过期节点"},
		{"tag": "测试节点 02"},
		{"tag": "🇯🇵 日本 03"},
	}
	got := dropExcluded(nodes, excluded)
	if len(got) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(got))
	}
	for _, n := range got {
		tag := n["tag"].(string)
		if strings.Contains(tag, "过期") || strings.Contains(tag, "测试") {
			t.Errorf("excluded node leaked through: %q", tag)
		}
	}
}

func TestDropExcluded_NoKeywords(t *testing.T) {
	var excluded []string
	nodes := []node.Node{
		{"tag": "节点 A"},
		{"tag": "节点 B"},
	}
	got := dropExcluded(nodes, excluded)
	if len(got) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(got))
	}
}

func TestDropExcluded_PreservesNoTagNodes(t *testing.T) {
	excluded := []string{"drop"}
	nodes := []node.Node{
		{"type": "direct"}, // no tag field
		{"tag": "drop me"},
		{"tag": "keep me"},
	}
	got := dropExcluded(nodes, excluded)
	if len(got) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(got))
	}
}

func TestDropExcluded_IgnoresEmojiInTag(t *testing.T) {
	excluded := []string{"美国"}
	nodes := []node.Node{
		{"tag": "🇺🇸 美国 01"}, // should be excluded
		{"tag": "🇯🇵 日本 02"}, // should be kept
	}
	got := dropExcluded(nodes, excluded)
	if len(got) != 1 {
		t.Fatalf("expected 1 node, got %d", len(got))
	}
	if got[0]["tag"] != "🇯🇵 日本 02" {
		t.Errorf("wrong node kept: %v", got[0]["tag"])
	}
}

func TestDropExcluded_EmptyInput(t *testing.T) {
	got := dropExcluded(nil, []string{"drop"})
	if len(got) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// prefixTags
// ---------------------------------------------------------------------------

func TestPrefixTags(t *testing.T) {
	nodes := []node.Node{
		{"tag": "节点 A"},
		{"tag": "节点 B"},
	}
	result := prefixTags(nodes, "mysub")
	for _, n := range result {
		tag := n["tag"].(string)
		if !strings.HasPrefix(tag, "[mysub] ") {
			t.Errorf("expected prefix [mysub], got %q", tag)
		}
	}
}

func TestPrefixTags_NoTagField(t *testing.T) {
	nodes := []node.Node{
		{"type": "direct"},
	}
	result := prefixTags(nodes, "sub")
	// node without tag should not be modified
	if _, ok := result[0]["tag"]; ok {
		t.Errorf("tag field should not have been added to node without tag")
	}
}

// ---------------------------------------------------------------------------
// stripTagEmoji
// ---------------------------------------------------------------------------

func TestStripTagEmoji(t *testing.T) {
	nodes := []node.Node{
		{"tag": "🇺🇸 美国 01"},
		{"tag": "🇯🇵 日本 02"},
		{"tag": "纯文字节点"},
	}
	result := stripTagEmoji(nodes)
	for _, n := range result {
		tag := n["tag"].(string)
		for _, r := range tag {
			if r > 0x2000 && r != ' ' {
				// rough check: no high-codepoint emoji should remain
				// (flags are in 0x1F1E0–0x1F1FF range)
				if r >= 0x1F1E0 && r <= 0x1F1FF {
					t.Errorf("emoji still present in tag %q", tag)
				}
			}
		}
	}
	// plain text node should be unchanged
	if result[2]["tag"] != "纯文字节点" {
		t.Errorf("plain text node was modified: %q", result[2]["tag"])
	}
}

// ---------------------------------------------------------------------------
// assignTagEmoji
// ---------------------------------------------------------------------------

func TestAssignTagEmoji_AddsFlag(t *testing.T) {
	nodes := []node.Node{
		{"tag": "美国 01"},
		{"tag": "日本 02"},
		{"tag": "香港 03"},
		{"tag": "新加坡 04"},
	}
	result := assignTagEmojiDefault(nodes)

	cases := []struct {
		idx  int
		flag string
	}{
		{0, "🇺🇸"},
		{1, "🇯🇵"},
		{2, "🇭🇰"},
		{3, "🇸🇬"},
	}
	for _, c := range cases {
		tag := result[c.idx]["tag"].(string)
		if !strings.HasPrefix(tag, c.flag) {
			t.Errorf("node[%d]: expected flag %s, got tag %q", c.idx, c.flag, tag)
		}
	}
}

func TestAssignTagEmoji_ReplacesExistingEmoji(t *testing.T) {
	nodes := []node.Node{
		{"tag": "🇯🇵 美国 01"}, // wrong flag, should be replaced with 🇺🇸
	}
	result := assignTagEmojiDefault(nodes)
	tag := result[0]["tag"].(string)
	if !strings.HasPrefix(tag, "🇺🇸") {
		t.Errorf("expected 🇺🇸 prefix, got %q", tag)
	}
	if strings.Contains(tag, "🇯🇵") {
		t.Errorf("old emoji 🇯🇵 should have been removed, got %q", tag)
	}
}

func TestAssignTagEmoji_UnknownRegionGetsDefaultFlag(t *testing.T) {
	nodes := []node.Node{
		{"tag": "未知地区节点"},
	}
	result := assignTagEmojiDefault(nodes)
	tag := result[0]["tag"].(string)
	// Should get the default UN flag
	if !strings.HasPrefix(tag, "🇺🇳") {
		t.Errorf("expected default 🇺🇳 flag, got %q", tag)
	}
}

// ---------------------------------------------------------------------------
// removeKeywords
// ---------------------------------------------------------------------------

func TestRemoveKeywords_PlainText(t *testing.T) {
	nodes := []node.Node{
		{"tag": "美国(112人) 01"},
		{"tag": "日本节点"},
	}
	result := removeKeywords(nodes, []string{"(112人)"})
	if result[0]["tag"] != "美国 01" {
		t.Errorf("expected '美国 01', got %q", result[0]["tag"])
	}
	if result[1]["tag"] != "日本节点" {
		t.Errorf("node without keyword should be unchanged, got %q", result[1]["tag"])
	}
}

func TestRemoveKeywords_GlobWildcard(t *testing.T) {
	nodes := []node.Node{
		{"tag": "美国(112人) 01"},
		{"tag": "日本(50人) 02"},
		{"tag": "香港节点"},
	}
	result := removeKeywords(nodes, []string{"(*人)"})
	if strings.Contains(result[0]["tag"].(string), "人") {
		t.Errorf("wildcard pattern should have removed '(112人)', got %q", result[0]["tag"])
	}
	if strings.Contains(result[1]["tag"].(string), "人") {
		t.Errorf("wildcard pattern should have removed '(50人)', got %q", result[1]["tag"])
	}
	if result[2]["tag"] != "香港节点" {
		t.Errorf("unmatched node should be unchanged, got %q", result[2]["tag"])
	}
}

func TestRemoveKeywords_QuestionMarkWildcard(t *testing.T) {
	// "节点?" converts to regex "节点." which matches any single char after "节点".
	// ReplaceAll removes all non-overlapping matches, so:
	//   "节点1"  → removes "节点1" → ""
	//   "节点A"  → removes "节点A" → ""
	//   "节点AB" → removes "节点A" (first match) → "B" (trimmed)
	nodes := []node.Node{
		{"tag": "节点1"},
		{"tag": "节点A"},
		{"tag": "节点AB"},
	}
	result := removeKeywords(nodes, []string{"节点?"})
	if result[0]["tag"] != "" {
		t.Errorf("expected empty tag for '节点1', got %q", result[0]["tag"])
	}
	if result[1]["tag"] != "" {
		t.Errorf("expected empty tag for '节点A', got %q", result[1]["tag"])
	}
	// "节点AB": "节点." matches "节点A", leaving "B"
	if result[2]["tag"] != "B" {
		t.Errorf("expected 'B' for '节点AB' after removing '节点?', got %q", result[2]["tag"])
	}
}

func TestRemoveKeywords_EmptyKeywords(t *testing.T) {
	nodes := []node.Node{
		{"tag": "节点 A"},
	}
	result := removeKeywords(nodes, nil)
	if result[0]["tag"] != "节点 A" {
		t.Errorf("empty keywords should not modify nodes, got %q", result[0]["tag"])
	}
}

func TestRemoveKeywords_MultipleKeywords(t *testing.T) {
	nodes := []node.Node{
		{"tag": "美国 IEPL 高速"},
	}
	result := removeKeywords(nodes, []string{"IEPL", "高速"})
	tag := result[0]["tag"].(string)
	if strings.Contains(tag, "IEPL") || strings.Contains(tag, "高速") {
		t.Errorf("both keywords should be removed, got %q", tag)
	}
}

// ---------------------------------------------------------------------------
// emoji_overrides
// ---------------------------------------------------------------------------

func TestEmojiTable_OverrideAddsARegion(t *testing.T) {
	table := newEmojiTable([]model.EmojiRule{
		{Emoji: "🇱🇺", Keywords: []string{"卢森堡", "LU"}},
	})

	nodes := assignTagEmoji([]node.Node{{"tag": "卢森堡 01"}}, table)
	if got := nodes[0].Tag(); got != "🇱🇺 卢森堡 01" {
		t.Errorf("tag = %q, want the override applied", got)
	}
	// The built-in table still works alongside it.
	nodes = assignTagEmoji([]node.Node{{"tag": "香港 02"}}, table)
	if got := nodes[0].Tag(); got != "🇭🇰 香港 02" {
		t.Errorf("tag = %q, want the built-in rule", got)
	}
}

func TestEmojiTable_OverrideBeatsBuiltin(t *testing.T) {
	// Operator entries are tried first, which is what makes overriding possible.
	table := newEmojiTable([]model.EmojiRule{
		{Emoji: "🏴󠁧󠁢󠁳󠁣󠁴󠁿", Keywords: []string{"英国", "UK"}},
	})

	nodes := assignTagEmoji([]node.Node{{"tag": "英国 01"}}, table)
	if got := nodes[0].Tag(); got != "🏴󠁧󠁢󠁳󠁣󠁴󠁿 英国 01" {
		t.Errorf("tag = %q, want the override to win over the built-in 🇬🇧", got)
	}
}

func TestEmojiTable_WholeWordMatchingAvoidsFalsePositives(t *testing.T) {
	table := newEmojiTable(nil)
	// "IN" sits inside "China"; a substring match would tag it as India.
	nodes := assignTagEmoji([]node.Node{{"tag": "China Telecom 01"}}, table)
	if got := nodes[0].Tag(); strings.HasPrefix(got, "🇮🇳") {
		t.Errorf("tag = %q, want no India match from the letters in China", got)
	}
}
