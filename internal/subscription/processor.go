package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

	"node-box/internal/logx"
	"node-box/internal/model"
	"node-box/internal/node"
	"node-box/internal/subscription/xray"
	"node-box/upstream/convert"
	upstreammodel "node-box/upstream/model"
	clashmodel "node-box/upstream/model/clash"

	"gopkg.in/yaml.v3"
)

// Processor parses one subscription format into nodes.
type Processor interface {
	Process(data []byte) ([]node.Node, error)
}

// ProcessorFor returns the processor for a subscription type.
func ProcessorFor(subType string) (Processor, error) {
	switch strings.ToLower(subType) {
	case model.SubClash:
		return clashProcessor{}, nil
	case model.SubSingBox:
		return singboxProcessor{}, nil
	case model.SubXray, model.SubV2Ray:
		return xray.Processor{}, nil
	default:
		return nil, fmt.Errorf("unsupported subscription type %q", subType)
	}
}

// clashProcessor converts a Clash YAML subscription using the vendored
// clash2singbox conversion in upstream/.
type clashProcessor struct{}

func (clashProcessor) Process(data []byte) ([]node.Node, error) {
	var cfg clashmodel.Clash
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse clash yaml: %w", err)
	}

	outbounds, endpoints, err := convert.Clash2sing(cfg, upstreammodel.SINGLATEST)
	if err != nil {
		// Conversion reports per-proxy failures as a multi-line error while
		// still returning everything it did manage to convert. Surface them
		// individually and keep the successful nodes.
		for line := range strings.SplitSeq(err.Error(), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				logx.Warnf("clash: skipped a proxy: %s", line)
			}
		}
	}

	nodes := make([]node.Node, 0, len(outbounds)+len(endpoints))
	for _, ob := range outbounds {
		if ob.Ignored {
			continue
		}
		n, err := toNode(ob)
		if err != nil {
			logx.Warnf("clash: skipped outbound %q: %v", ob.Tag, err)
			continue
		}
		nodes = append(nodes, n)
	}
	for _, ep := range endpoints {
		n, err := toNode(ep)
		if err != nil {
			logx.Warnf("clash: skipped endpoint %q: %v", ep.Tag, err)
			continue
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// toNode round-trips a typed upstream struct through JSON into a generic node.
func toNode(v any) (node.Node, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	var n node.Node
	if err := json.Unmarshal(data, &n); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return n, nil
}

// singboxProcessor extracts proxy outbounds from a native sing-box config.
type singboxProcessor struct{}

// nonProxyTypes are sing-box outbound types that are routing constructs rather
// than actual proxies, so they are never imported as nodes.
var nonProxyTypes = map[string]bool{
	"direct":   true,
	"block":    true,
	"selector": true,
	"urltest":  true,
	"dns":      true,
}

func (singboxProcessor) Process(data []byte) ([]node.Node, error) {
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse sing-box json: %w", err)
	}

	var nodes []node.Node
	for _, section := range []string{"outbounds", "endpoints"} {
		raw, ok := cfg[section]
		if !ok {
			continue
		}
		arr, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an array", section)
		}
		for _, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			n := node.Node(m)
			if t := n.Type(); t == "" || nonProxyTypes[t] {
				continue
			}
			nodes = append(nodes, n)
		}
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no proxy nodes found in outbounds or endpoints")
	}
	return nodes, nil
}

// The xray package implements Processor itself, so there is nothing to wrap.
var _ Processor = xray.Processor{}
