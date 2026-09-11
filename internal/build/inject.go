package build

import (
	"fmt"

	"node-box/internal/logx"
	"node-box/internal/model"
	"node-box/internal/subscription"
	"node-box/internal/textutil"
)

// relayArrow joins a relay template tag to its upstream tag.
const relayArrow = " → "

// endpointTypes are sing-box outbound types that must live under "endpoints"
// rather than "outbounds".
var endpointTypes = map[string]bool{
	"wireguard": true,
	"tailscale": true,
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
	pool []subscription.Node
	// relays maps a relay name to the nodes it generates.
	relays map[string][]subscription.Node
	// byTag indexes every candidate node, regular and generated alike.
	byTag map[string]subscription.Node
	// modules indexes modules by name so a config's rules can be found.
	modules map[string]model.Module
}

func newInjector(cfg *model.Config, nodes map[string][]subscription.Node) (*injector, error) {
	inj := &injector{
		cfg:     cfg,
		relays:  make(map[string][]subscription.Node),
		byTag:   make(map[string]subscription.Node),
		modules: make(map[string]model.Module, len(cfg.Modules)),
	}
	for _, m := range cfg.Modules {
		inj.modules[m.Name] = m
	}

	// Walk subscriptions in declaration order so the pool order is stable.
	for _, sub := range cfg.Nodes.Subscriptions {
		if !sub.Enable {
			continue
		}
		for _, n := range nodes[sub.Name] {
			if n.Tag() == "" {
				continue
			}
			inj.pool = append(inj.pool, n)
			inj.byTag[n.Tag()] = n
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

		var generated []subscription.Node
		seen := make(map[string]bool)
		for _, tmpl := range templates {
			for _, up := range upstreams {
				tag := tmpl.Tag() + relayArrow + up.Tag()
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
func (inj *injector) resolveUnion(sels []model.NodeSelector) []subscription.Node {
	matched := make(map[string]bool)
	for _, sel := range sels {
		for _, n := range inj.resolve(sel) {
			matched[n.Tag()] = true
		}
	}
	// Emit in pool order rather than match order so the result does not depend
	// on how the selectors were written.
	var out []subscription.Node
	for _, n := range inj.pool {
		if matched[n.Tag()] {
			out = append(out, n)
		}
	}
	return out
}

// resolve returns the nodes matching one selector.
func (inj *injector) resolve(sel model.NodeSelector) []subscription.Node {
	if sel.IsEmpty() {
		return nil
	}
	from := make(map[string]bool, len(sel.From))
	for _, name := range sel.From {
		from[name] = true
	}

	var out []subscription.Node
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

// members returns the nodes one rule puts into its selector, regular nodes
// first and then each referenced relay in declaration order.
func (inj *injector) members(rule model.SelectorRule) []subscription.Node {
	out := inj.resolve(rule.NodeSelector)
	for _, name := range rule.Relays {
		out = append(out, inj.relays[name]...)
	}
	return out
}
