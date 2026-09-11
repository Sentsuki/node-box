package subscription_test

import (
	"strings"
	"testing"

	"node-box/internal/subscription"
)

// ---------------------------------------------------------------------------
// Filter.FilterNodes
// ---------------------------------------------------------------------------

func TestFilter_FilterNodes_ExcludesMatchingTags(t *testing.T) {
	f := subscription.NewFilter([]string{"过期", "测试"})
	nodes := []subscription.Node{
		{"tag": "🇺🇸 美国 01"},
		{"tag": "过期节点"},
		{"tag": "测试节点 02"},
		{"tag": "🇯🇵 日本 03"},
	}
	got := f.FilterNodes(nodes)
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

func TestFilter_FilterNodes_NoKeywords(t *testing.T) {
	f := subscription.NewFilter(nil)
	nodes := []subscription.Node{
		{"tag": "节点 A"},
		{"tag": "节点 B"},
	}
	got := f.FilterNodes(nodes)
	if len(got) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(got))
	}
}

func TestFilter_FilterNodes_PreservesNoTagNodes(t *testing.T) {
	f := subscription.NewFilter([]string{"drop"})
	nodes := []subscription.Node{
		{"type": "direct"}, // no tag field
		{"tag": "drop me"},
		{"tag": "keep me"},
	}
	got := f.FilterNodes(nodes)
	if len(got) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(got))
	}
}

func TestFilter_FilterNodes_IgnoresEmojiInTag(t *testing.T) {
	f := subscription.NewFilter([]string{"美国"})
	nodes := []subscription.Node{
		{"tag": "🇺🇸 美国 01"}, // should be excluded
		{"tag": "🇯🇵 日本 02"}, // should be kept
	}
	got := f.FilterNodes(nodes)
	if len(got) != 1 {
		t.Fatalf("expected 1 node, got %d", len(got))
	}
	if got[0]["tag"] != "🇯🇵 日本 02" {
		t.Errorf("wrong node kept: %v", got[0]["tag"])
	}
}

func TestFilter_FilterNodes_EmptyInput(t *testing.T) {
	f := subscription.NewFilter([]string{"drop"})
	got := f.FilterNodes(nil)
	if len(got) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// AddSubscriptionPrefix
// ---------------------------------------------------------------------------

func TestAddSubscriptionPrefix(t *testing.T) {
	nodes := []subscription.Node{
		{"tag": "节点 A"},
		{"tag": "节点 B"},
	}
	result := subscription.AddSubscriptionPrefix(nodes, "mysub")
	for _, n := range result {
		tag := n["tag"].(string)
		if !strings.HasPrefix(tag, "[mysub] ") {
			t.Errorf("expected prefix [mysub], got %q", tag)
		}
	}
}

func TestAddSubscriptionPrefix_NoTagField(t *testing.T) {
	nodes := []subscription.Node{
		{"type": "direct"},
	}
	result := subscription.AddSubscriptionPrefix(nodes, "sub")
	// node without tag should not be modified
	if _, ok := result[0]["tag"]; ok {
		t.Errorf("tag field should not have been added to node without tag")
	}
}

// ---------------------------------------------------------------------------
// RemoveEmoji
// ---------------------------------------------------------------------------

func TestRemoveEmoji(t *testing.T) {
	nodes := []subscription.Node{
		{"tag": "🇺🇸 美国 01"},
		{"tag": "🇯🇵 日本 02"},
		{"tag": "纯文字节点"},
	}
	result := subscription.RemoveEmoji(nodes)
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
// AutoEmoji
// ---------------------------------------------------------------------------

func TestAutoEmoji_AddsFlag(t *testing.T) {
	nodes := []subscription.Node{
		{"tag": "美国 01"},
		{"tag": "日本 02"},
		{"tag": "香港 03"},
		{"tag": "新加坡 04"},
	}
	result := subscription.AutoEmoji(nodes)

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

func TestAutoEmoji_ReplacesExistingEmoji(t *testing.T) {
	nodes := []subscription.Node{
		{"tag": "🇯🇵 美国 01"}, // wrong flag, should be replaced with 🇺🇸
	}
	result := subscription.AutoEmoji(nodes)
	tag := result[0]["tag"].(string)
	if !strings.HasPrefix(tag, "🇺🇸") {
		t.Errorf("expected 🇺🇸 prefix, got %q", tag)
	}
	if strings.Contains(tag, "🇯🇵") {
		t.Errorf("old emoji 🇯🇵 should have been removed, got %q", tag)
	}
}

func TestAutoEmoji_UnknownRegionGetsDefaultFlag(t *testing.T) {
	nodes := []subscription.Node{
		{"tag": "未知地区节点"},
	}
	result := subscription.AutoEmoji(nodes)
	tag := result[0]["tag"].(string)
	// Should get the default UN flag
	if !strings.HasPrefix(tag, "🇺🇳") {
		t.Errorf("expected default 🇺🇳 flag, got %q", tag)
	}
}

// ---------------------------------------------------------------------------
// RemoveKeywords
// ---------------------------------------------------------------------------

func TestRemoveKeywords_PlainText(t *testing.T) {
	nodes := []subscription.Node{
		{"tag": "美国(112人) 01"},
		{"tag": "日本节点"},
	}
	result := subscription.RemoveKeywords(nodes, []string{"(112人)"})
	if result[0]["tag"] != "美国 01" {
		t.Errorf("expected '美国 01', got %q", result[0]["tag"])
	}
	if result[1]["tag"] != "日本节点" {
		t.Errorf("node without keyword should be unchanged, got %q", result[1]["tag"])
	}
}

func TestRemoveKeywords_GlobWildcard(t *testing.T) {
	nodes := []subscription.Node{
		{"tag": "美国(112人) 01"},
		{"tag": "日本(50人) 02"},
		{"tag": "香港节点"},
	}
	result := subscription.RemoveKeywords(nodes, []string{"(*人)"})
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
	nodes := []subscription.Node{
		{"tag": "节点1"},
		{"tag": "节点A"},
		{"tag": "节点AB"},
	}
	result := subscription.RemoveKeywords(nodes, []string{"节点?"})
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
	nodes := []subscription.Node{
		{"tag": "节点 A"},
	}
	result := subscription.RemoveKeywords(nodes, nil)
	if result[0]["tag"] != "节点 A" {
		t.Errorf("empty keywords should not modify nodes, got %q", result[0]["tag"])
	}
}

func TestRemoveKeywords_MultipleKeywords(t *testing.T) {
	nodes := []subscription.Node{
		{"tag": "美国 IEPL 高速"},
	}
	result := subscription.RemoveKeywords(nodes, []string{"IEPL", "高速"})
	tag := result[0]["tag"].(string)
	if strings.Contains(tag, "IEPL") || strings.Contains(tag, "高速") {
		t.Errorf("both keywords should be removed, got %q", tag)
	}
}
