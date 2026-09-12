package build

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"node-box/internal/model"
	"node-box/internal/node"
)

// The fixture mirrors the shape of a real setup: some self-hosted nodes, two
// airports, and a subscription whose nodes are only ever used as relay
// templates.
func fixtureNodes() map[string][]node.Node {
	n := func(tag, typ string) node.Node {
		return node.Node{"tag": tag, "type": typ, "server": "example.com"}
	}
	return map[string][]node.Node{
		"A": {
			n("[A] node-1", "vmess"),
			n("[A] node-2", "vmess"),
		},
		"hi": {
			n("[hi] 🇺🇸 美国 01", "vless"),
			n("[hi] 🇭🇰 香港 01", "vless"),
		},
		"mj": {
			n("[mj] 🇭🇰 香港 02", "trojan"),
			n("[mj] 🇯🇵 日本 01", "trojan"),
		},
		"RL": {
			n("[RL] US", "vmess"),
			n("[RL] JP", "vmess"),
			n("[RL] WARP", "wireguard"),
		},
	}
}

func sel(from ...string) model.NodeSelector {
	return model.NodeSelector{From: from}
}

func fixtureConfig(rules []model.SelectorRule) *model.Config {
	return &model.Config{
		Nodes: &model.NodesConfig{
			Subscriptions: []model.Subscription{
				{Name: "A", Path: "nodes/a.json", Type: model.SubSingBox, Enable: true},
				{Name: "hi", URL: "https://hi.example.com", Type: model.SubSingBox, Enable: true},
				{Name: "mj", URL: "https://mj.example.com", Type: model.SubClash, Enable: true},
				{Name: "RL", Path: "nodes/relay.json", Type: model.SubSingBox, Enable: true},
			},
			Relays: []model.Relay{
				{
					Name: "US",
					Via:  []model.NodeSelector{{From: []string{"RL"}, Include: []string{"US"}}},
					Upstream: []model.NodeSelector{
						{From: []string{"hi"}, Include: []string{"美国"}},
						{From: []string{"mj"}, Include: []string{"香港"}},
					},
				},
				{
					Name:     "JP",
					Via:      []model.NodeSelector{{From: []string{"RL"}, Include: []string{"JP"}}},
					Upstream: []model.NodeSelector{{From: []string{"mj"}, Include: []string{"日本", "香港"}}},
				},
				{
					Name:     "JP-HK",
					Via:      []model.NodeSelector{{From: []string{"RL"}, Include: []string{"JP"}}},
					Upstream: []model.NodeSelector{{From: []string{"mj"}, Include: []string{"香港"}}},
				},
				{
					Name:     "WARP",
					Via:      []model.NodeSelector{{From: []string{"RL"}, Include: []string{"WARP"}}},
					Upstream: []model.NodeSelector{{From: []string{"mj"}, Include: []string{"日本"}}},
				},
			},
		},
		Modules: []model.Module{
			{Name: "out", File: "modules/out.json", Selectors: rules},
		},
		Configs:        []model.ConfigFile{{Name: "main", Path: "main.json", Modules: []string{"out"}}},
		UpdateSchedule: &model.Schedule{Type: model.ScheduleHourly},
	}
}

// moduleFile is a realistic outbounds module: a fixed direct outbound plus two
// groups, one of which already lists a literal member.
const moduleFile = `{
  "outbounds": [
    { "type": "direct",   "tag": "direct" },
    { "type": "selector", "tag": "Proxy", "outbounds": ["direct"] },
    { "type": "selector", "tag": "AI" }
  ]
}`

// buildFixture runs Build over the fixture and returns the decoded document.
func buildFixture(t *testing.T, rules []model.SelectorRule) map[string]any {
	t.Helper()
	files, err := Build(Input{
		Config:  fixtureConfig(rules),
		Modules: mods(map[string]string{"out": moduleFile}),
		Nodes:   fixtureNodes(),
		Outputs: outputs(model.ConfigFile{Name: "main", Path: "main.json", Modules: []string{"out"}}),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(files[0].Content, &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	return doc
}

// members returns a group's member list.
func groupMembers(t *testing.T, doc map[string]any, tag string) []string {
	t.Helper()
	arr, _ := doc["outbounds"].([]any)
	obj := findByTag(arr, tag)
	if obj == nil {
		t.Fatalf("no outbound tagged %q", tag)
	}
	raw, _ := obj["outbounds"].([]any)
	out := make([]string, 0, len(raw))
	for _, m := range raw {
		out = append(out, m.(string))
	}
	return out
}

// sectionTags returns the tags of every entry in a section.
func sectionTags(doc map[string]any, section string) []string {
	arr, _ := doc[section].([]any)
	var out []string
	for _, raw := range arr {
		if obj, ok := raw.(map[string]any); ok {
			if tag, ok := obj["tag"].(string); ok {
				out = append(out, tag)
			}
		}
	}
	return out
}

func TestInject_MembershipDrivesInsertion(t *testing.T) {
	doc := buildFixture(t, []model.SelectorRule{
		{Tag: "Proxy", NodeSelector: sel("A"), Relays: []string{"US"}},
		{Tag: "AI", Relays: []string{"JP-HK"}},
	})

	// One ordering rule everywhere: the module's own entries, then relays in
	// nodes.relays order, then regular nodes in nodes.subscriptions order.
	want := []string{
		"direct",
		"[RL] US [hi] 🇺🇸 美国 01",
		"[RL] US [mj] 🇭🇰 香港 02",
		"[A] node-1",
		"[A] node-2",
	}
	if got := groupMembers(t, doc, "Proxy"); !slices.Equal(got, want) {
		t.Errorf("Proxy members:\n got %q\nwant %q", got, want)
	}

	// AI had no literal members and asks only for a relay.
	if got := groupMembers(t, doc, "AI"); !slices.Equal(got, []string{"[RL] JP [mj] 🇭🇰 香港 02"}) {
		t.Errorf("AI members = %q", got)
	}

	inserted := sectionTags(doc, "outbounds")

	// Nodes nobody references must not be written at all. This is the whole
	// point of the inversion: there is nothing to subtract afterwards.
	for _, tag := range []string{
		"[hi] 🇭🇰 香港 01", // not selected by any rule or relay
		"[mj] 🇯🇵 日本 01", // upstream of JP, but JP is not referenced
		"[RL] US",       // a template, never a node in its own right
		"[RL] JP",
		"[RL] WARP",
	} {
		if slices.Contains(inserted, tag) {
			t.Errorf("%q should not have been inserted", tag)
		}
	}

	// Detour targets must be present even though no selector lists them.
	for _, tag := range []string{"[hi] 🇺🇸 美国 01", "[mj] 🇭🇰 香港 02"} {
		if !slices.Contains(inserted, tag) {
			t.Errorf("%q is a detour target and must be inserted", tag)
		}
	}
}

func TestInject_RelayUnionIsNotACrossProduct(t *testing.T) {
	doc := buildFixture(t, []model.SelectorRule{
		{Tag: "Proxy", NodeSelector: sel("A"), Relays: []string{"US"}},
		{Tag: "AI", Relays: []string{"US"}},
	})

	// US upstream is (hi ∩ 美国) ∪ (mj ∩ 香港). A single selector over
	// {hi,mj} × {美国,香港} would also produce [hi] 香港 and [mj] 美国.
	want := []string{
		"[RL] US [hi] 🇺🇸 美国 01",
		"[RL] US [mj] 🇭🇰 香港 02",
	}
	if got := groupMembers(t, doc, "AI"); !slices.Equal(got, want) {
		t.Errorf("US relay nodes:\n got %q\nwant %q", got, want)
	}
}

func TestInject_OverlappingRelayDefinitionsShareNodes(t *testing.T) {
	doc := buildFixture(t, []model.SelectorRule{
		{Tag: "Proxy", Relays: []string{"JP"}},
		{Tag: "AI", Relays: []string{"JP-HK"}},
	})

	// JP-HK is a subset of JP. A definition is a named selection over the
	// template-upstream space, not a generation event, so the shared pair
	// appears once in the file.
	inserted := sectionTags(doc, "outbounds")
	count := 0
	for _, tag := range inserted {
		if tag == "[RL] JP [mj] 🇭🇰 香港 02" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("the shared relay node appears %d times, want 1", count)
	}

	if got := groupMembers(t, doc, "AI"); !slices.Equal(got, []string{"[RL] JP [mj] 🇭🇰 香港 02"}) {
		t.Errorf("AI members = %q", got)
	}
	if got := len(groupMembers(t, doc, "Proxy")); got != 3 { // direct + 2 JP nodes
		t.Errorf("Proxy has %d members, want 3", got)
	}
}

func TestInject_EmptyFromMeansNoRegularNodes(t *testing.T) {
	doc := buildFixture(t, []model.SelectorRule{
		{Tag: "Proxy", NodeSelector: sel("A")},
		{Tag: "AI", Relays: []string{"US"}},
	})

	// AI declared no "from", so it gets relays only.
	for _, m := range groupMembers(t, doc, "AI") {
		if !strings.HasPrefix(m, "[RL] ") {
			t.Errorf("AI should contain only relay nodes, found %q", m)
		}
	}
}

func TestInject_IncludeExcludeIgnoreEmoji(t *testing.T) {
	doc := buildFixture(t, []model.SelectorRule{
		{Tag: "Proxy", NodeSelector: model.NodeSelector{
			From:    []string{"hi", "mj"},
			Include: []string{"香港"},
		}},
		{Tag: "AI", NodeSelector: model.NodeSelector{
			From:    []string{"hi", "mj"},
			Exclude: []string{"香港"},
		}},
	})

	if got := groupMembers(t, doc, "Proxy"); !slices.Equal(got,
		[]string{"direct", "[hi] 🇭🇰 香港 01", "[mj] 🇭🇰 香港 02"}) {
		t.Errorf("Proxy members = %q", got)
	}
	if got := groupMembers(t, doc, "AI"); !slices.Equal(got,
		[]string{"[hi] 🇺🇸 美国 01", "[mj] 🇯🇵 日本 01"}) {
		t.Errorf("AI members = %q", got)
	}
}

func TestInject_WireguardGoesToEndpoints(t *testing.T) {
	doc := buildFixture(t, []model.SelectorRule{
		{Tag: "Proxy", NodeSelector: sel("A")},
		{Tag: "AI", Relays: []string{"WARP"}},
	})

	// The WARP template is a wireguard node, so its generated relay belongs
	// under endpoints while still being referenced from a selector.
	wg := "[RL] WARP [mj] 🇯🇵 日本 01"
	if got := sectionTags(doc, "endpoints"); !slices.Contains(got, wg) {
		t.Errorf("endpoints = %q, want it to contain %q", got, wg)
	}
	if got := sectionTags(doc, "outbounds"); slices.Contains(got, wg) {
		t.Error("a wireguard node must not stay in outbounds")
	}
	if got := groupMembers(t, doc, "AI"); !slices.Equal(got, []string{wg}) {
		t.Errorf("AI members = %q", got)
	}
}

func TestInject_UnknownSelectorTagIsAnError(t *testing.T) {
	_, err := Build(Input{
		Config: fixtureConfig([]model.SelectorRule{
			{Tag: "Nope", NodeSelector: sel("A")},
		}),
		Modules: mods(map[string]string{"out": moduleFile}),
		Nodes:   fixtureNodes(),
		Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"out"}}),
	})
	if err == nil || !strings.Contains(err.Error(), "Nope") {
		t.Fatalf("want an error naming the missing selector, got %v", err)
	}
}

func TestInject_GroupLeftEmptyIsRejected(t *testing.T) {
	// "AI" has no literal members and the rule matches nothing, so the group
	// would be emitted empty. sing-box refuses to start on that, and the old
	// implementation wrote it as null.
	_, err := Build(Input{
		Config: fixtureConfig([]model.SelectorRule{
			{Tag: "Proxy", NodeSelector: sel("A")},
			{Tag: "AI", NodeSelector: model.NodeSelector{
				From:    []string{"A"},
				Include: []string{"does-not-exist"},
			}},
		}),
		Modules: mods(map[string]string{"out": moduleFile}),
		Nodes:   fixtureNodes(),
		Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"out"}}),
	})
	if err == nil || !strings.Contains(err.Error(), "no members") {
		t.Fatalf("want an empty-group error, got %v", err)
	}
}

func TestInject_RelayWithNoTemplateIsAnError(t *testing.T) {
	cfg := fixtureConfig([]model.SelectorRule{{Tag: "Proxy", NodeSelector: sel("A")}})
	cfg.Nodes.Relays[0].Via = []model.NodeSelector{{From: []string{"RL"}, Include: []string{"no-such-template"}}}

	_, err := Build(Input{
		Config:  cfg,
		Modules: mods(map[string]string{"out": moduleFile}),
		Nodes:   fixtureNodes(),
		Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"out"}}),
	})
	if err == nil || !strings.Contains(err.Error(), "via matched no nodes") {
		t.Fatalf("want a relay resolution error, got %v", err)
	}
}

func TestInject_IsDeterministic(t *testing.T) {
	rules := []model.SelectorRule{
		{Tag: "Proxy", NodeSelector: model.NodeSelector{From: []string{"A", "hi", "mj"}}, Relays: []string{"US", "JP"}},
		{Tag: "AI", Relays: []string{"JP-HK", "WARP"}},
	}
	in := Input{
		Config:  fixtureConfig(rules),
		Modules: mods(map[string]string{"out": moduleFile}),
		Nodes:   fixtureNodes(),
		Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"out"}}),
	}

	first, err := Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for i := range 20 {
		again, err := Build(Input{
			Config:  fixtureConfig(rules),
			Modules: mods(map[string]string{"out": moduleFile}),
			Nodes:   fixtureNodes(),
			Outputs: in.Outputs,
		})
		if err != nil {
			t.Fatalf("Build %d: %v", i, err)
		}
		if first[0].Hash() != again[0].Hash() {
			t.Fatalf("run %d differs:\n%s\n---\n%s", i, first[0].Content, again[0].Content)
		}
	}
}

func TestInject_SharedModuleAcrossOutputs(t *testing.T) {
	// The same module feeds two outputs; neither may see the other's edits.
	rules := []model.SelectorRule{
		{Tag: "Proxy", NodeSelector: sel("A")},
		{Tag: "AI", Relays: []string{"US"}},
	}
	cfg := fixtureConfig(rules)
	cfg.Configs = []model.ConfigFile{
		{Name: "one", Path: "one.json", Modules: []string{"out"}},
		{Name: "two", Path: "two.json", Modules: []string{"out"}},
	}

	files, err := Build(Input{
		Config:  cfg,
		Modules: mods(map[string]string{"out": moduleFile}),
		Nodes:   fixtureNodes(),
		Outputs: outputs(cfg.Configs[0], cfg.Configs[1]),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if string(files[0].Content) != string(files[1].Content) {
		t.Errorf("two outputs from the same module diverged:\n%s\n---\n%s", files[0].Content, files[1].Content)
	}
}

func TestInject_OrderingRule(t *testing.T) {
	// The rule lists relays in the opposite order to nodes.relays and
	// subscriptions out of declaration order; neither may affect the output.
	doc := buildFixture(t, []model.SelectorRule{
		{
			Tag:          "Proxy",
			NodeSelector: model.NodeSelector{From: []string{"mj", "A"}},
			Relays:       []string{"JP", "US"},
		},
		{Tag: "AI", NodeSelector: sel("A")},
	})

	want := []string{
		"direct", // the module file's own member
		// relays, in nodes.relays order: US before JP
		"[RL] US [hi] 🇺🇸 美国 01",
		"[RL] US [mj] 🇭🇰 香港 02",
		"[RL] JP [mj] 🇭🇰 香港 02",
		"[RL] JP [mj] 🇯🇵 日本 01",
		// regular nodes, in nodes.subscriptions order: A before mj
		"[A] node-1",
		"[A] node-2",
		"[mj] 🇭🇰 香港 02",
		"[mj] 🇯🇵 日本 01",
	}
	if got := groupMembers(t, doc, "Proxy"); !slices.Equal(got, want) {
		t.Errorf("Proxy members:\n got %q\nwant %q", got, want)
	}

	// The outbounds array follows the same rule, after the module's entries.
	tags := sectionTags(doc, "outbounds")
	firstRelay, firstRegular := -1, -1
	for i, tag := range tags {
		if firstRelay < 0 && strings.HasPrefix(tag, "[RL] ") {
			firstRelay = i
		}
		if firstRegular < 0 && strings.HasPrefix(tag, "[A] ") {
			firstRegular = i
		}
	}
	if firstRelay < 0 || firstRegular < 0 || firstRelay > firstRegular {
		t.Errorf("relays must precede regular nodes in outbounds, got %q", tags)
	}
}

func TestInject_OpenVPNGoesToEndpoints(t *testing.T) {
	// The vendored Clash converter emits "openvpn-client" as an endpoint, so
	// node-box must route it like wireguard. Both the old implementation and
	// the first version of this one only knew about wireguard/tailscale.
	nodes := fixtureNodes()
	nodes["RL"] = append(nodes["RL"], node.Node{
		"tag": "[RL] OVPN", "type": "openvpn-client", "server": "vpn.example.com",
	})

	cfg := fixtureConfig([]model.SelectorRule{
		{Tag: "Proxy", NodeSelector: sel("A")},
		{Tag: "AI", Relays: []string{"OVPN"}},
	})
	cfg.Nodes.Relays = append(cfg.Nodes.Relays, model.Relay{
		Name:     "OVPN",
		Via:      []model.NodeSelector{{From: []string{"RL"}, Include: []string{"OVPN"}}},
		Upstream: []model.NodeSelector{{From: []string{"mj"}, Include: []string{"日本"}}},
	})

	files, err := Build(Input{
		Config:  cfg,
		Modules: mods(map[string]string{"out": moduleFile}),
		Nodes:   nodes,
		Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"out"}}),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var doc map[string]any
	json.Unmarshal(files[0].Content, &doc)

	ovpn := "[RL] OVPN [mj] 🇯🇵 日本 01"
	if got := sectionTags(doc, "endpoints"); !slices.Contains(got, ovpn) {
		t.Errorf("endpoints = %q, want it to contain %q", got, ovpn)
	}
	if got := sectionTags(doc, "outbounds"); slices.Contains(got, ovpn) {
		t.Error("an openvpn-client node must not stay in outbounds")
	}
}

func TestValidate_RejectsEndpointTypeInOutbounds(t *testing.T) {
	// A hand-written wireguard outbound in a module file. Derived nodes are
	// routed by type, so this can only come from the module itself, and
	// node-box reports it rather than quietly relocating it.
	const bad = `{
      "outbounds": [
        { "type": "direct", "tag": "direct" },
        { "type": "wireguard", "tag": "wg-manual", "address": ["10.0.0.2/32"] },
        { "type": "selector", "tag": "Proxy", "outbounds": ["direct"] }
      ]
    }`

	_, err := Build(Input{
		Config:  fixtureConfig([]model.SelectorRule{{Tag: "Proxy", NodeSelector: sel("A")}}),
		Modules: mods(map[string]string{"out": bad}),
		Nodes:   fixtureNodes(),
		Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"out"}}),
	})
	if err == nil {
		t.Fatal("want an error for a wireguard outbound")
	}
	for _, want := range []string{"wg-manual", "wireguard", "endpoints"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

func TestInject_DuplicateTagsCollapseConsistently(t *testing.T) {
	// A provider shipping the same name twice used to be resolved two different
	// ways: insertion kept the first node, detour lookup found the last.
	cfg := &model.Config{
		Nodes: &model.NodesConfig{
			Subscriptions: []model.Subscription{
				{Name: "own", URL: "u", Type: model.SubSingBox, Enable: true},
			},
		},
		Modules: []model.Module{{
			Name: "m", File: "m.json",
			Selectors: []model.SelectorRule{{Tag: "Proxy", NodeSelector: model.NodeSelector{From: []string{"own"}}}},
		}},
		Configs:        []model.ConfigFile{{Name: "main", Path: "main.json", Modules: []string{"m"}}},
		UpdateSchedule: &model.Schedule{Type: model.ScheduleHourly},
	}
	nodes := map[string][]node.Node{"own": {
		{"type": "vmess", "tag": "[own] dup", "server": "first"},
		{"type": "vmess", "tag": "[own] dup", "server": "second"},
	}}

	inj, err := newInjector(cfg, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if len(inj.pool) != 1 {
		t.Fatalf("pool has %d nodes, want the duplicate dropped", len(inj.pool))
	}
	// The surviving node and the one detour resolution finds must be the same.
	if got := inj.pool[0]["server"]; got != "first" {
		t.Errorf("pool kept server %v, want the first", got)
	}
	if got := inj.byTag["[own] dup"]["server"]; got != "first" {
		t.Errorf("byTag resolves to server %v, want the same node the pool kept", got)
	}
}
