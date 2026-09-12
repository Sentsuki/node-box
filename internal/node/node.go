// Package node holds the one representation of a sing-box proxy node that every
// other package agrees on.
//
// It lives on its own so that the subscription parsers, the assembler and the
// format-specific converters can all speak about nodes without depending on one
// another. Before this package existed, the xray converter returned bare maps
// and the subscription layer converted them one by one into its own identical
// type, which is the kind of parallel abstraction that quietly drifts.
package node

// Node is a single sing-box outbound (or endpoint) as a generic JSON object.
// Keeping it untyped means fields node-box does not know about survive a
// round trip untouched.
type Node map[string]any

// Tag returns the node's tag, or "" if it is missing or not a string.
func (n Node) Tag() string {
	tag, _ := n["tag"].(string)
	return tag
}

// SetTag sets the node's tag.
func (n Node) SetTag(tag string) { n["tag"] = tag }

// Type returns the node's type, or "" if it is missing or not a string.
func (n Node) Type() string {
	t, _ := n["type"].(string)
	return t
}

// Detour returns the tag this node chains through, or "" if it is direct.
func (n Node) Detour() string {
	d, _ := n["detour"].(string)
	return d
}

// SetDetour points this node at an upstream node.
func (n Node) SetDetour(tag string) { n["detour"] = tag }

// Clone returns a deep copy of the node.
//
// A shallow copy is not enough: outbounds routinely carry nested objects such
// as tls, transport and multiplex. Copies that shared those would let a later
// edit of one node silently change its siblings.
func (n Node) Clone() Node {
	return Node(cloneMap(n))
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneValue(v)
	}
	return out
}

func cloneValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return cloneMap(t)
	case []any:
		s := make([]any, len(t))
		for i, e := range t {
			s[i] = cloneValue(e)
		}
		return s
	default:
		// Everything else coming out of encoding/json is an immutable scalar.
		return v
	}
}
