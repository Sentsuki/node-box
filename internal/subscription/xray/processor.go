// Package xray parses Xray/V2Ray subscriptions: base64-wrapped lists of
// sharing links (vmess://, vless://, ss://, trojan://) converted to sing-box
// nodes.
package xray

import (
	"fmt"
	"strings"

	"node-box/internal/logx"
	"node-box/internal/node"
)

// Processor parses an Xray/V2Ray subscription.
//
// It satisfies subscription.Processor directly. Returning bare maps for the
// caller to convert one by one, as this used to, made the node representation
// something two packages each defined for themselves.
type Processor struct{}

// Process decodes the subscription and converts every sharing link it holds.
//
// A link that cannot be parsed is logged and skipped; only a subscription where
// nothing at all converted is an error, since a provider adding one unsupported
// protocol should not cost the operator every other node.
func (Processor) Process(data []byte) ([]node.Node, error) {
	lines := splitLines(decodeSubscription(data))
	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid links found in subscription data")
	}

	var nodes []node.Node
	var errs []string

	for _, line := range lines {
		n, err := parseLink(line)
		if err != nil {
			errs = append(errs, err.Error())
			logx.Warnf("xray: skipped a link: %s", err)
			continue
		}
		nodes = append(nodes, node.Node(n))
	}

	if len(nodes) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("all links failed to convert: %s", strings.Join(errs, "; "))
	}
	return nodes, nil
}
