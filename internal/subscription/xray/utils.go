// Package xray provides subscription data processing for Xray/V2Ray standard
// sharing link format. It supports base64-encoded subscription content and
// converts standard proxy sharing URIs directly into SingBox outbound nodes.
package xray

import (
	"encoding/base64"
	"strings"
)

// tryBase64Decode attempts to decode a base64-encoded string.
// It handles standard, URL-safe, padded and unpadded variants.
func tryBase64Decode(s string) (string, bool) {
	// Try standard base64 with padding
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return string(b), true
	}
	// Try standard base64 without padding
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return string(b), true
	}
	// Try URL-safe base64 with padding
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return string(b), true
	}
	// Try URL-safe base64 without padding
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return string(b), true
	}
	return "", false
}

// isLikelyShareLinks checks if data looks like raw (non-base64) sharing links.
func isLikelyShareLinks(data string) bool {
	for _, prefix := range []string{"vmess://", "vless://", "ss://", "trojan://"} {
		if strings.HasPrefix(data, prefix) {
			return true
		}
	}
	return false
}

// decodeSubscription decodes subscription data, auto-detecting base64 encoding.
func decodeSubscription(data []byte) string {
	s := strings.TrimSpace(string(data))

	// If it already looks like sharing links, return as-is
	if isLikelyShareLinks(s) {
		return s
	}

	// Try base64 decoding
	if decoded, ok := tryBase64Decode(s); ok {
		return decoded
	}

	// Fallback: return as-is
	return s
}

// splitLines splits a string into lines, filtering out empty lines.
func splitLines(s string) []string {
	raw := strings.Split(s, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// queryBool returns true if the query parameter matches "1" or "true".
func queryBool(val string) bool {
	return val == "1" || strings.ToLower(val) == "true"
}
