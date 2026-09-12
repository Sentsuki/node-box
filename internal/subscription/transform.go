package subscription

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"node-box/internal/logx"
	"node-box/internal/model"
	"node-box/internal/node"
	"node-box/internal/textutil"
)

// The functions in this file are the naming pipeline applied to one
// subscription's nodes, in the order fetchOne applies them.
//
// They all edit the nodes in place and return the slice for readability. That is
// safe because a subscription's nodes are freshly parsed and owned by the
// fetcher until they are handed to the assembler, and it avoids copying every
// node four times for what amounts to four string edits. Nothing outside this
// package calls them, which is why they are unexported: they are steps, not an
// API.

// dropExcluded removes nodes whose tag matches any of the keywords.
//
// This is source cleanup rather than selection: subscriptions routinely carry
// entries that are not nodes at all ("套餐到期：...", "官网：..."), and those must
// never enter the pool. A node with no tag is kept, because the keywords have
// nothing to match against and dropping it would be a guess.
func dropExcluded(nodes []node.Node, keywords []string) []node.Node {
	if len(keywords) == 0 {
		return nodes
	}

	kept := make([]node.Node, 0, len(nodes))
	dropped := 0

	for _, n := range nodes {
		tag := n.Tag()
		if tag == "" {
			kept = append(kept, n)
			continue
		}
		if matchesAny(tag, keywords) {
			dropped++
			continue
		}
		kept = append(kept, n)
	}

	if dropped > 0 {
		logx.Debugf("excluded %d node(s) by keyword", dropped)
	}
	return kept
}

// matchesAny reports whether tag contains any of the keywords, ignoring emoji.
func matchesAny(tag string, keywords []string) bool {
	for _, kw := range keywords {
		if textutil.ContainsIgnoreEmoji(tag, kw) {
			return true
		}
	}
	return false
}

// prefixTags marks every node with the subscription it came from, as
// "[name] original".
//
// The prefix is what makes a tag unique across subscriptions and is how
// selectors attribute a node to its source, which is why subscription names may
// not contain brackets.
func prefixTags(nodes []node.Node, subName string) []node.Node {
	for _, n := range nodes {
		if tag := n.Tag(); tag != "" {
			n.SetTag(fmt.Sprintf("[%s] %s", subName, tag))
		}
	}
	return nodes
}

// stripTagEmoji removes emoji from every tag.
func stripTagEmoji(nodes []node.Node) []node.Node {
	for _, n := range nodes {
		if tag := n.Tag(); tag != "" {
			n.SetTag(strings.TrimSpace(removeEmojiRunes(tag)))
		}
	}
	return nodes
}

// assignTagEmoji replaces whatever emoji a tag carries with one chosen from the
// node's name, so a mixed set of providers ends up with one consistent scheme.
func assignTagEmoji(nodes []node.Node, table emojiTable) []node.Node {
	for _, n := range nodes {
		tag := n.Tag()
		if tag == "" {
			continue
		}
		clean := strings.TrimSpace(removeEmojiRunes(tag))
		n.SetTag(table.forTag(clean) + " " + clean)
	}
	return nodes
}

// removeEmojiRunes drops every emoji code point, using the same detector the
// keyword comparison uses so the two can never disagree about what an emoji is.
func removeEmojiRunes(s string) string {
	return strings.Map(func(r rune) rune {
		if textutil.IsEmojiRune(r) {
			return -1
		}
		return r
	}, s)
}

// removeKeywords deletes substrings from every tag.
//
// Patterns may use glob wildcards — * for any run of characters, ? for exactly
// one — because the things worth deleting are usually templated rather than
// fixed: "(123人)", "剩余 12.3GB", "到期 2026-01-01".
func removeKeywords(nodes []node.Node, keywords []string) []node.Node {
	matchers := compileRemovals(keywords)
	if len(matchers) == 0 {
		return nodes
	}

	for _, n := range nodes {
		tag := n.Tag()
		if tag == "" {
			continue
		}
		for _, m := range matchers {
			if m.re != nil {
				tag = m.re.ReplaceAllString(tag, "")
			} else {
				tag = strings.ReplaceAll(tag, m.literal, "")
			}
		}
		n.SetTag(collapseSpaces(tag))
	}
	return nodes
}

// removal is one compiled deletion pattern: either a literal or a glob.
type removal struct {
	literal string
	re      *regexp.Regexp
}

func compileRemovals(keywords []string) []removal {
	var out []removal
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if !strings.ContainsAny(kw, "*?") {
			out = append(out, removal{literal: kw})
			continue
		}
		re, err := globToRegexp(kw)
		if err != nil {
			// Fall back to a literal rather than dropping the rule: the operator
			// asked for something to be removed, and removing it verbatim is
			// closer to that than removing nothing.
			logx.Warnf("remove_keywords pattern %q did not compile (%v); treating it as literal text", kw, err)
			out = append(out, removal{literal: kw})
			continue
		}
		out = append(out, removal{re: re})
	}
	return out
}

// globToRegexp converts a glob with * and ? into a regexp, escaping everything
// else so a keyword containing regexp syntax means itself.
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	for _, ch := range pattern {
		switch ch {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	return regexp.Compile(b.String())
}

// collapseSpaces squeezes runs of spaces and trims the result, tidying the gaps
// that deleting a substring leaves behind.
func collapseSpaces(s string) string {
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// --- region emoji ----------------------------------------------------------

// emojiTable maps node names to a region emoji.
//
// Entries are tried in order and the first hit wins, so the table is a list
// rather than a map. Operator-supplied entries are placed ahead of the built-in
// ones, which is what lets a config both add regions and override them.
type emojiTable []emojiRule

type emojiRule struct {
	emoji    string
	keywords []string
}

// unknownRegionEmoji marks a node whose name matched nothing.
const unknownRegionEmoji = "🇺🇳"

// newEmojiTable builds the lookup table for one run.
func newEmojiTable(overrides []model.EmojiRule) emojiTable {
	table := make(emojiTable, 0, len(overrides)+len(builtinEmojiRules))
	for _, o := range overrides {
		table = append(table, emojiRule{emoji: o.Emoji, keywords: o.Keywords})
	}
	return append(table, builtinEmojiRules...)
}

// forTag returns the emoji for a node name.
func (t emojiTable) forTag(tag string) string {
	upper := strings.ToUpper(tag)
	for _, rule := range t {
		for _, kw := range rule.keywords {
			if kw != "" && containsWord(upper, strings.ToUpper(kw)) {
				return rule.emoji
			}
		}
	}
	return unknownRegionEmoji
}

// containsWord reports whether s contains keyword as a whole word.
//
// Plain substring matching is not usable here: two-letter country codes appear
// inside ordinary words constantly, so "IN" would tag every "China" node as
// India. What counts as a boundary depends on the script:
//   - a CJK keyword ("英国") is bounded by any non-CJK character, so digits and
//     spaces both end it
//   - an ASCII keyword ("UK") is bounded by any non-letter, so "UK3" still
//     matches while "UKRAINE" does not
func containsWord(s, keyword string) bool {
	idx := strings.Index(s, keyword)
	if idx == -1 {
		return false
	}

	first, _ := utf8.DecodeRuneInString(keyword)
	cjk := isCJKRune(first)

	isBoundary := func(r rune) bool {
		if cjk {
			return !isCJKRune(r)
		}
		return !isASCIILetter(r)
	}

	if idx > 0 {
		prev, _ := utf8.DecodeLastRuneInString(s[:idx])
		if !isBoundary(prev) {
			return false
		}
	}
	if end := idx + len(keyword); end < len(s) {
		next, _ := utf8.DecodeRuneInString(s[end:])
		if !isBoundary(next) {
			return false
		}
	}
	return true
}

// isCJKRune reports whether r is a CJK Unified Ideograph.
func isCJKRune(r rune) bool { return r >= 0x4E00 && r <= 0x9FFF }

// isASCIILetter reports whether r is an ASCII letter.
func isASCIILetter(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

// builtinEmojiRules covers the regions airports actually sell. It is a starting
// point, not a policy: nodes.emoji_overrides extends and overrides it without a
// rebuild, which is why this list does not try to be exhaustive.
var builtinEmojiRules = emojiTable{
	{"🇦🇷", []string{"阿根廷", "AR", "Argentina"}},
	{"🇦🇺", []string{"澳大利亚", "澳洲", "AU", "Australia"}},
	{"🇦🇹", []string{"奥地利", "AT", "Austria"}},
	{"🇧🇩", []string{"孟加拉", "BD", "Bangladesh"}},
	{"🇧🇪", []string{"比利时", "BE", "Belgium"}},
	{"🇧🇷", []string{"巴西", "BR", "Brazil"}},
	{"🇨🇦", []string{"加拿大", "CA", "Canada"}},
	{"🇨🇱", []string{"智利", "CL", "Chile"}},
	{"🇨🇴", []string{"哥伦比亚", "CO", "Colombia"}},
	{"🇨🇿", []string{"捷克", "CZ", "Czech"}},
	{"🇩🇰", []string{"丹麦", "DK", "Denmark"}},
	{"🇪🇬", []string{"埃及", "EG", "Egypt"}},
	{"🇫🇮", []string{"芬兰", "FI", "Finland"}},
	{"🇫🇷", []string{"法国", "FR", "France"}},
	{"🇩🇪", []string{"德国", "DE", "Germany"}},
	{"🇭🇰", []string{"香港", "HK", "Hong Kong", "HongKong"}},
	{"🇭🇺", []string{"匈牙利", "HU", "Hungary"}},
	{"🇮🇸", []string{"冰岛", "IS", "Iceland"}},
	{"🇮🇳", []string{"印度", "IN", "India"}},
	{"🇮🇩", []string{"印尼", "印度尼西亚", "ID", "Indonesia"}},
	{"🇮🇪", []string{"爱尔兰", "IE", "Ireland"}},
	{"🇮🇱", []string{"以色列", "IL", "Israel"}},
	{"🇮🇹", []string{"意大利", "IT", "Italy"}},
	{"🇯🇵", []string{"日本", "JP", "Japan"}},
	{"🇰🇿", []string{"哈萨克斯坦", "KZ", "Kazakhstan"}},
	{"🇰🇪", []string{"肯尼亚", "KE", "Kenya"}},
	{"🇰🇷", []string{"韩国", "KR", "Korea"}},
	{"🇲🇾", []string{"马来西亚", "MY", "大马", "Malaysia"}},
	{"🇲🇽", []string{"墨西哥", "MX", "Mexico"}},
	{"🇳🇱", []string{"荷兰", "NL", "Netherlands"}},
	{"🇳🇿", []string{"新西兰", "NZ", "New Zealand"}},
	{"🇳🇬", []string{"尼日利亚", "NG", "Nigeria"}},
	{"🇳🇴", []string{"挪威", "NO", "Norway"}},
	{"🇵🇰", []string{"巴基斯坦", "PK", "Pakistan"}},
	{"🇵🇭", []string{"菲律宾", "PH", "Philippines"}},
	{"🇵🇱", []string{"波兰", "PL", "Poland"}},
	{"🇵🇹", []string{"葡萄牙", "PT", "Portugal"}},
	{"🇷🇴", []string{"罗马尼亚", "RO", "Romania"}},
	{"🇷🇺", []string{"俄罗斯", "RU", "Russia"}},
	{"🇸🇦", []string{"沙特", "SA", "Saudi"}},
	{"🇸🇬", []string{"新加坡", "SG", "Singapore"}},
	{"🇿🇦", []string{"南非", "ZA", "South Africa"}},
	{"🇪🇸", []string{"西班牙", "ES", "Spain"}},
	{"🇸🇪", []string{"瑞典", "SE", "Sweden"}},
	{"🇨🇭", []string{"瑞士", "CH", "Switzerland"}},
	{"🇹🇼", []string{"台湾", "TW", "Taiwan"}},
	{"🇹🇭", []string{"泰国", "TH", "Thailand"}},
	{"🇹🇷", []string{"土耳其", "TR", "Turkey", "Türkiye"}},
	{"🇦🇪", []string{"阿联酋", "AE", "UAE", "Dubai", "迪拜"}},
	{"🇺🇦", []string{"乌克兰", "UA", "Ukraine"}},
	{"🇬🇧", []string{"英国", "UK", "GB", "United Kingdom", "Britain"}},
	{"🇺🇸", []string{"美国", "US", "USA", "United States", "America"}},
	{"🇻🇳", []string{"越南", "VN", "Vietnam"}},
}
