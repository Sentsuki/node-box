package build

import (
	"fmt"

	"node-box/internal/logx"
	"node-box/internal/model"
	"node-box/internal/node"
)

// inject fills in selector membership for one output and writes the nodes
// those selectors reference.
func (inj *injector) inject(doc map[string]any, cf model.ConfigFile) error {
	rules := inj.rulesFor(cf)
	if len(rules) == 0 {
		return nil
	}

	outbounds, ok := doc["outbounds"].([]any)
	if !ok {
		return fmt.Errorf("selector rules are configured but the assembled configuration has no outbounds array")
	}

	present := documentTags(doc)
	needed := make(map[string]bool)

	for _, rule := range rules {
		selector := findByTag(outbounds, rule.Tag)
		if selector == nil {
			return fmt.Errorf("selector rule references tag %q, which no module in this output declares", rule.Tag)
		}

		members := inj.members(rule)
		if len(members) == 0 {
			// Not fatal on its own: the selector may still have literal
			// members. An empty selector is caught by validateDocument.
			logx.Warnf("selector %q in output %q: no nodes matched", rule.Tag, cf.Name)
		}

		addMembers(selector, members)
		for _, n := range members {
			needed[n.Tag()] = true
		}
	}

	if err := inj.closeOverDetours(needed, present); err != nil {
		return err
	}
	inj.insert(doc, needed, present)
	return nil
}

// closeOverDetours adds the upstream of every chained node to the needed set.
//
// A relay node is useless without the node its detour points at, and a
// selector that references only relays would otherwise produce a configuration
// full of dangling detours.
func (inj *injector) closeOverDetours(needed map[string]bool, present map[string]bool) error {
	queue := make([]string, 0, len(needed))
	for tag := range needed {
		queue = append(queue, tag)
	}

	for len(queue) > 0 {
		tag := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		node, known := inj.byTag[tag]
		if !known {
			continue
		}
		detour, _ := node["detour"].(string)
		if detour == "" || needed[detour] {
			continue
		}
		if _, candidate := inj.byTag[detour]; candidate {
			needed[detour] = true
			queue = append(queue, detour)
			continue
		}
		if present[detour] {
			// Points at something the module file already declares, such as
			// a fixed direct outbound.
			continue
		}
		return fmt.Errorf("node %q has detour %q, which is not a known node", tag, detour)
	}
	return nil
}

// insert writes the needed nodes into the document.
//
// Emission follows the single ordering rule used everywhere: relay nodes
// first in nodes.relays order, then regular nodes in nodes.subscriptions
// order. Both are declaration orders, so the generated file is
// byte-identical across runs with the same inputs.
func (inj *injector) insert(doc map[string]any, needed, present map[string]bool) {
	var addedOutbounds, addedEndpoints []any

	for _, n := range inj.candidates() {
		tag := n.Tag()
		if !needed[tag] || present[tag] {
			continue
		}
		present[tag] = true

		// A copy, because the pool is shared across every output in this build.
		clone := map[string]any(n.Clone())
		if endpointTypes[n.Type()] {
			addedEndpoints = append(addedEndpoints, clone)
		} else {
			addedOutbounds = append(addedOutbounds, clone)
		}
	}

	if len(addedOutbounds) > 0 {
		doc["outbounds"] = append(doc["outbounds"].([]any), addedOutbounds...)
	}
	if len(addedEndpoints) > 0 {
		existing, _ := doc["endpoints"].([]any)
		doc["endpoints"] = append(existing, addedEndpoints...)
	}
	logx.Debugf("inserted %d outbound(s) and %d endpoint(s)", len(addedOutbounds), len(addedEndpoints))
}

// candidates returns every node that could be inserted, in a stable order:
// relays first, then regular nodes.
func (inj *injector) candidates() []node.Node {
	out := make([]node.Node, 0, len(inj.pool))
	for _, r := range inj.cfg.Nodes.Relays {
		out = append(out, inj.relays[r.Name]...)
	}
	return append(out, inj.pool...)
}

// findByTag returns the object in outbounds carrying the given tag.
func findByTag(outbounds []any, tag string) map[string]any {
	for _, raw := range outbounds {
		obj, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := obj["tag"].(string); ok && t == tag {
			return obj
		}
	}
	return nil
}

// addMembers appends node tags to a selector, after whatever the module file
// listed itself. Literal entries keep their position and meaning; derived ones
// follow in a predictable order.
func addMembers(selector map[string]any, members []node.Node) {
	existing, _ := selector["outbounds"].([]any)

	listed := make(map[string]bool, len(existing))
	for _, raw := range existing {
		if s, ok := raw.(string); ok {
			listed[s] = true
		}
	}

	// Start from a fresh slice so the module's own array is never aliased.
	result := make([]any, 0, len(existing)+len(members))
	result = append(result, existing...)
	for _, n := range members {
		if tag := n.Tag(); !listed[tag] {
			listed[tag] = true
			result = append(result, tag)
		}
	}
	selector["outbounds"] = result
}

// documentTags collects the tags already present in a document.
func documentTags(doc map[string]any) map[string]bool {
	tags := make(map[string]bool)
	for _, section := range []string{"outbounds", "endpoints"} {
		arr, ok := doc[section].([]any)
		if !ok {
			continue
		}
		for _, raw := range arr {
			if obj, ok := raw.(map[string]any); ok {
				if tag, ok := obj["tag"].(string); ok && tag != "" {
					tags[tag] = true
				}
			}
		}
	}
	return tags
}
