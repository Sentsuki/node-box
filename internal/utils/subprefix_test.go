// Package utils_test contains targeted tests for the sub-prefix + emoji scenario.
package utils_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"node-box/internal/utils"
)

// stripEmojiLocal mirrors match.go's stripEmoji — used here to print debug output.
func stripEmojiLocal(s string) string {
	var b strings.Builder
	prevWasSpace := false
	for _, r := range s {
		isSo := unicode.Is(unicode.So, r)
		isRI := r >= 0x1F1E0 && r <= 0x1F1FF
		isVS := (r >= 0xFE00 && r <= 0xFE0F) || (r >= 0xE0100 && r <= 0xE01EF)
		isZWJ := r == 0x200D
		isKeycap := r == 0x20E3
		isSkin := r >= 0x1F3FB && r <= 0x1F3FF
		if isSo || isRI || isVS || isZWJ || isKeycap || isSkin {
			if !prevWasSpace {
				b.WriteRune(' ')
				prevWasSpace = true
			}
			continue
		}
		prevWasSpace = r == ' ' || r == '\t'
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// TestSubPrefixWithEmojiMatch verifies that "[sub] 美国" as a keyword correctly
// matches node tags in both emoji:true and emoji:false modes.
func TestSubPrefixWithEmojiMatch(t *testing.T) {
	// These are realistic node tags AFTER AddSubscriptionPrefix is applied.
	tags := []struct {
		mode string
		tag  string // as stored in the node after processing
	}{
		// emoji:true  → AutoEmoji runs → strips existing emoji, adds flag, THEN prefix added
		{"emoji:true", "[sub] 🇺🇸 美国 01"},
		{"emoji:true", "[sub] 🇺🇸 美国 Premium"},
		// emoji:false → RemoveEmoji runs → strips emoji, THEN prefix added
		{"emoji:false", "[sub] 美国 01"},
		{"emoji:false", "[sub] 美国 Premium"},
		// Should NOT match (different country)
		{"emoji:true", "[sub] 🇯🇵 日本 02"},
		{"emoji:false", "[sub] 日本 02"},
	}

	keywords := []struct {
		kw          string
		shouldMatch []string // which modes should match
	}{
		// Plain keyword without prefix
		{"美国", []string{"emoji:true", "emoji:false"}},
		// Keyword with sub prefix, no emoji
		{"[sub] 美国", []string{"emoji:true", "emoji:false"}},
		// Keyword with sub prefix AND emoji
		{"[sub] 🇺🇸 美国", []string{"emoji:true", "emoji:false"}},
	}

	for _, kw := range keywords {
		t.Run(fmt.Sprintf("keyword=%q", kw.kw), func(t *testing.T) {
			wantMatchModes := make(map[string]bool)
			for _, m := range kw.shouldMatch {
				wantMatchModes[m] = true
			}

			for _, tc := range tags {
				got := utils.ContainsIgnoreEmoji(tc.tag, kw.kw)
				shouldMatch := wantMatchModes[tc.mode] && (strings.Contains(tc.tag, "美国"))

				if got != shouldMatch {
					// Print debug info
					cleanTag := stripEmojiLocal(tc.tag)
					cleanKw := stripEmojiLocal(kw.kw)
					t.Errorf(
						"[%s] tag=%q, keyword=%q\n"+
							"  stripEmoji(tag)=%q\n"+
							"  stripEmoji(kw) =%q\n"+
							"  got=%v, want=%v",
						tc.mode, tc.tag, kw.kw,
						cleanTag, cleanKw,
						got, shouldMatch,
					)
				}
			}
		})
	}
}
