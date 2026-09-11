package build

import (
	"node-box/internal/model"
	"node-box/internal/subscription"
)

// NodeInjector places subscription nodes into a configuration document's
// outbounds and endpoints sections.
//
// This is the seam where the still-undesigned half of node-box plugs in. The
// open questions are:
//
//   - how subscription nodes are inserted into outbounds
//   - how selector and urltest membership is computed
//   - how relay nodes are expanded (the current "full cartesian product, then
//     filter" approach does not scale)
//   - how wireguard and tailscale outbounds move into endpoints
//   - where output-level filtering such as no_need_nodes belongs
//
// Until those are settled, PassthroughInjector is used and the generated
// configuration contains exactly the outbounds and endpoints its modules
// declared.
type NodeInjector interface {
	Inject(doc map[string]any, cf model.ConfigFile, nodes map[string][]subscription.Node) error
}

// PassthroughInjector leaves outbounds and endpoints exactly as the modules
// defined them.
type PassthroughInjector struct{}

// Inject does nothing.
func (PassthroughInjector) Inject(map[string]any, model.ConfigFile, map[string][]subscription.Node) error {
	return nil
}
