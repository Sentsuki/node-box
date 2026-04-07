// Package utils provides shared utility functions for node-box.
package utils

import (
	"strings"
	"unicode"
)

// stripEmoji removes all emoji (Symbol, Other category) and regional indicator
// symbols from s, then collapses runs of whitespace into a single space and
// trims leading/trailing whitespace.
// The result is used only for comparison; the original string is never modified.
func stripEmoji(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevWasSpace := false
	for _, r := range s {
		if isEmojiRune(r) {
			// Mark that we saw non-text here; a single space will be
			// inserted if the surrounding text needs to be separated.
			// We DON'T write anything here — the surrounding spaces in
			// the original string already provide word separation.
			prevWasSpace = true
			continue
		}
		isSpace := r == ' ' || r == '\t'
		if isSpace {
			if !prevWasSpace {
				b.WriteRune(' ')
			}
			prevWasSpace = true
		} else {
			b.WriteRune(r)
			prevWasSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// isEmojiRune reports whether r is an emoji or emoji-related code point.
// Covers:
//   - Unicode category So (Symbol, Other) – flags, pictographs, etc.
//   - Regional Indicator symbols (U+1F1E0–U+1F1FF) used in flag sequences.
//   - Variation selectors (U+FE00–U+FE0F, U+E0100–U+E01EF).
//   - Zero Width Joiner (U+200D) and combining enclosing keycaps (U+20E3).
func isEmojiRune(r rune) bool {
	if unicode.Is(unicode.So, r) {
		return true
	}
	switch {
	case r >= 0x1F1E0 && r <= 0x1F1FF: // Regional Indicator
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // Variation selectors
		return true
	case r >= 0xE0100 && r <= 0xE01EF: // Variation selectors supplement
		return true
	case r == 0x200D: // Zero Width Joiner
		return true
	case r == 0x20E3: // Combining Enclosing Keycap
		return true
	case r >= 0x1F3FB && r <= 0x1F3FF: // Skin tone modifiers
		return true
	}
	return false
}

// ContainsIgnoreEmoji reports whether s contains keyword, ignoring emoji in
// both strings during comparison.
//
// Example:
//
//	ContainsIgnoreEmoji("🇺🇸 美国 01", "美国")  → true
//	ContainsIgnoreEmoji("美国 01", "🇺🇸 美国")  → true
//
// The original strings are never modified.
func ContainsIgnoreEmoji(s, keyword string) bool {
	if keyword == "" {
		return false
	}
	cleanS := stripEmoji(s)
	cleanKeyword := stripEmoji(keyword)
	if cleanKeyword == "" {
		// keyword was pure emoji; fall back to literal match
		return strings.Contains(s, keyword)
	}
	return strings.Contains(cleanS, cleanKeyword)
}
