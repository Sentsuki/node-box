package xray

import (
	"fmt"
	"strings"

	"node-box/internal/logger"
)

// XrayProcessor handles Xray/V2Ray subscription data processing.
// It decodes base64 subscription content and converts standard
// sharing links (vmess://, vless://, ss://, trojan://) to SingBox outbound nodes.
type XrayProcessor struct{}

// NewXrayProcessor creates a new Xray processor instance.
func NewXrayProcessor() *XrayProcessor {
	return &XrayProcessor{}
}

// Process handles Xray subscription data and converts to unified Node format.
// It auto-detects base64 encoding, splits lines, and converts each sharing link.
func (xp *XrayProcessor) Process(data []byte) ([]map[string]any, error) {
	decoded := decodeSubscription(data)
	lines := splitLines(decoded)

	if len(lines) == 0 {
		return nil, fmt.Errorf("xray: no valid links found in subscription data")
	}

	var nodes []map[string]any
	var errs []string

	for _, line := range lines {
		node, err := parseLink(line)
		if err != nil {
			errs = append(errs, err.Error())
			logger.Warn("Xray conversion skipped: %s", err)
			continue
		}
		nodes = append(nodes, node)
	}

	if len(nodes) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("xray: all links failed to convert: %s", strings.Join(errs, "; "))
	}

	return nodes, nil
}
