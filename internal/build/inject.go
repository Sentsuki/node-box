package build

import (
	"fmt"

	"node-box/internal/logx"
	"node-box/internal/model"
	"node-box/internal/node"
	"node-box/internal/textutil"
)

// relayTagSeparator joins a relay template tag to its upstream tag.
const relayTagSeparator = " "

// endpointTypes are node types that must live under "endpoints" rather than
// "outbounds".
//
// This list has to cover everything the vendored Clash converter can emit as an
// endpoint — see the typeMap in upstream/convert/convert.go, which turns Clash
// "wireguard" into "wireguard" and Clash "openvpn" into "openvpn-client" —
// plus the types a sing-box subscription can declare in its own endpoints array.
var endpointTypes = map[string]bool{
	"wireguard":      true,
	"tailscale":      true,
	"openvpn-client": true,
}

// injector turns selector rules into concrete configuration.
//
// Selector membership is the single source of truth: a node is written to a
// generated file exactly when some rule references it. Nothing is inserted
// speculatively, so nothing has to be subtracted afterwards.
type injector struct {
	cfg *model.Config

	// pool holds every regular node in configuration order, which is what
	// makes the generated member lists deterministic.
	pool []node.Node
	// relays maps a relay name to the nodes it generates.
	relays map[string][]node.Node
	// byTag indexes every candidate node, regular and generated alike.
	byTag map[string]node.Node
	// modules indexes modules by name so a config's rules can be found.
	modules map[string]model.Module
}

func newInjector(cfg *model.Config, nodes map[string][]node.Node) (*injector, error) {
	inj := &injector{
		cfg:     cfg,
		relays:  make(map[string][]node.Node),
		byTag:   make(map[string]node.Node),
		modules: make(map[string]model.Module, len(cfg.Modules)),
	}
	for _, m := range cfg.Modules {
		inj.modules[m.Name] = m
	}

	// Walk subscriptions in declaration order so the pool order is stable.
	//
	// A tag may only enter the pool once. Providers do ship the same name twice,
	// and keeping both used to give two different answers to the same question:
	// insertion emitted the first node while detour resolution looked up the
	// last. One rule, applied here, is what keeps those consistent.
	for _, sub := range cfg.Nodes.Subscriptions {
		if !sub.Enable {
			continue
		}
		for _, n := range nodes[sub.Name] {
			tag := n.Tag()
			if tag == "" {
				continue
			}
			if _, dup := inj.byTag[tag]; dup {
				logx.Warnf("subscription %q: dropping a second node tagged %q; keeping the first", sub.Name, tag)
				continue
			}
			inj.pool = append(inj.pool, n)
			inj.byTag[tag] = n
		}
	}

	if err := inj.buildRelays(); err != nil {
		return nil, err
	}
	return inj, nil
}

// buildRelays materialises every relay definition.
//
// Generation is bounded by the declaration: only the templates and upstreams
// actually selected are paired. Nothing is produced and then filtered away.
func (inj *injector) buildRelays() error {
	for _, r := range inj.cfg.Nodes.Relays {
		templates := inj.resolveUnion(r.Via)
		if len(templates) == 0 {
			return fmt.Errorf("relay %q: via matched no nodes", r.Name)
		}
		upstreams := inj.resolveUnion(r.Upstream)
		if len(upstreams) == 0 {
			return fmt.Errorf("relay %q: upstream matched no nodes", r.Name)
		}

		var generated []node.Node
		seen := make(map[string]bool)
		for _, tmpl := range templates {
			for _, up := range upstreams {
				tag := tmpl.Tag() + relayTagSeparator + up.Tag()
				// Two definitions may overlap; a definition is a named
				// selection over the template-upstream space, not a
				// generation event, so identical pairs collapse.
				if seen[tag] {
					continue
				}
				seen[tag] = true

				n := tmpl.Clone()
				n.SetTag(tag)
				n["detour"] = up.Tag()

				generated = append(generated, n)
				if _, taken := inj.byTag[tag]; !taken {
					inj.byTag[tag] = n
				}
			}
		}
		inj.relays[r.Name] = generated
		logx.Debugf("relay %q: %d template(s) × %d upstream(s) = %d nodes",
			r.Name, len(templates), len(upstreams), len(generated))
	}
	return nil
}

// resolveUnion returns the union of several selectors, in pool order and
// without duplicates.
func (inj *injector) resolveUnion(sels []model.NodeSelector) []node.Node {
	matched := make(map[string]bool)
	for _, sel := range sels {
		for _, n := range inj.resolve(sel) {
			matched[n.Tag()] = true
		}
	}
	// Emit in pool order rather than match order so the result does not depend
	// on how the selectors were written.
	var out []node.Node
	for _, n := range inj.pool {
		if matched[n.Tag()] {
			out = append(out, n)
		}
	}
	return out
}

// resolve returns the nodes matching one selector.
func (inj *injector) resolve(sel model.NodeSelector) []node.Node {
	if sel.IsEmpty() {
		return nil
	}
	from := make(map[string]bool, len(sel.From))
	for _, name := range sel.From {
		from[name] = true
	}

	var out []node.Node
	for _, sub := range inj.cfg.Nodes.Subscriptions {
		if !sub.Enable || !from[sub.Name] {
			continue
		}
		for _, n := range inj.pool {
			if !belongsTo(n.Tag(), sub.Name) {
				continue
			}
			if matches(n.Tag(), sel.Include, sel.Exclude) {
				out = append(out, n)
			}
		}
	}
	return out
}

// belongsTo reports whether a tag carries the given subscription's prefix.
func belongsTo(tag, subName string) bool {
	prefix := "[" + subName + "] "
	return len(tag) >= len(prefix) && tag[:len(prefix)] == prefix
}

// matches applies include and exclude keyword lists to a tag.
func matches(tag string, include, exclude []string) bool {
	if len(include) > 0 {
		hit := false
		for _, kw := range include {
			if kw != "" && textutil.ContainsIgnoreEmoji(tag, kw) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	for _, kw := range exclude {
		if kw != "" && textutil.ContainsIgnoreEmoji(tag, kw) {
			return false
		}
	}
	return true
}

// rulesFor returns the selector rules that apply to one output, in the order
// its modules are listed.
func (inj *injector) rulesFor(cf model.ConfigFile) []model.SelectorRule {
	var rules []model.SelectorRule
	for _, name := range cf.Modules {
		rules = append(rules, inj.modules[name].Selectors...)
	}
	return rules
}

// members returns the nodes one rule puts into its selector.
//
// Relays come first, ordered by their nodes.relays declaration rather than by
// how the rule happens to list them, so the same set always appears in the
// same order. Regular nodes follow in nodes.subscriptions order.
func (inj *injector) members(rule model.SelectorRule) []node.Node {
	wanted := make(map[string]bool, len(rule.Relays))
	for _, name := range rule.Relays {
		wanted[name] = true
	}

	var out []node.Node
	for _, r := range inj.cfg.Nodes.Relays {
		if wanted[r.Name] {
			out = append(out, inj.relays[r.Name]...)
		}
	}
	return append(out, inj.resolve(rule.NodeSelector)...)
}
