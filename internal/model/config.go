package model

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// Subscription types.
const (
	SubClash   = "clash"
	SubSingBox = "singbox"
	SubXray    = "xray"
	SubV2Ray   = "v2ray"
)

// Schedule types.
const (
	ScheduleInterval = "interval"
	ScheduleHourly   = "hourly"
)

// Config is the configuration that lives in the config repository (config.json).
type Config struct {
	Output         *OutputConfig `json:"output,omitempty"`
	Nodes          *NodesConfig  `json:"nodes"`
	Modules        []Module      `json:"modules"`
	Configs        []ConfigFile  `json:"configs"`
	UpdateSchedule *Schedule     `json:"update_schedule"`
	UserAgent      string        `json:"user_agent,omitempty"`
}

// OutputConfig controls where generated files are written.
type OutputConfig struct {
	// Dir is the base directory for relative configs[].path values.
	// Relative values are resolved against the bootstrap root.
	Dir string `json:"dir,omitempty"`
}

// NodesConfig holds subscription sources, global filtering and relay definitions.
type NodesConfig struct {
	Subscriptions []Subscription `json:"subscriptions"`

	// ExcludeKeywords drops nodes whose tag matches, at fetch time.
	//
	// This is source cleanup, not selection: subscriptions routinely carry
	// entries that are not nodes at all ("套餐到期：...", "官网：..."). Those
	// must never enter the node pool, and no per-selector rule can express
	// that without repeating itself everywhere.
	ExcludeKeywords []string `json:"exclude_keywords,omitempty"`

	// Relays are named chained-proxy definitions.
	Relays []Relay `json:"relays,omitempty"`

	// EmojiOverrides extends and overrides the built-in region emoji table used
	// when a subscription sets emoji: true.
	//
	// It is configuration rather than code because the set of regions an operator
	// cares about changes with their providers, and needing a rebuild to tag a
	// new country is the wrong trade. Entries are tried before the built-in ones,
	// so naming a region that already exists replaces it.
	EmojiOverrides []EmojiRule `json:"emoji_overrides,omitempty"`
}

// EmojiRule assigns one emoji to every node whose name contains any of the
// keywords. Matching is whole-word and ignores case.
type EmojiRule struct {
	Emoji    string   `json:"emoji"`
	Keywords []string `json:"keywords"`
}

// NodeSelector picks a subset of the node pool.
//
// The same shape is used everywhere nodes are chosen: selector membership,
// relay templates and relay upstreams.
type NodeSelector struct {
	// From lists subscription names. Empty means no regular nodes at all,
	// which is how a selector says "chained proxies only".
	From []string `json:"from,omitempty"`
	// Include keeps only tags containing any of these. Empty keeps everything
	// From matched. Comparison ignores emoji on both sides.
	Include []string `json:"include,omitempty"`
	// Exclude drops tags containing any of these.
	Exclude []string `json:"exclude,omitempty"`
}

// IsEmpty reports whether the selector can never match anything.
func (s NodeSelector) IsEmpty() bool { return len(s.From) == 0 }

// Relay is a named set of chained proxies.
//
// Each template in Via is paired with each node in Upstream, producing one
// node per pair whose detour points at the upstream. Both fields are lists of
// selectors combined as a union, because an upstream set is often
// "this subscription's US nodes plus that subscription's HK nodes" — which a
// single selector would widen into a cross product.
type Relay struct {
	Name     string         `json:"name"`
	Via      []NodeSelector `json:"via"`
	Upstream []NodeSelector `json:"upstream"`
}

// SelectorRule fills in the members of one selector or urltest outbound.
//
// Membership is the single source of truth: a node is written to the generated
// configuration exactly when some rule references it.
type SelectorRule struct {
	// Tag identifies the selector in the module file.
	Tag string `json:"tag"`
	// NodeSelector picks regular nodes. Its fields appear inline in JSON.
	NodeSelector
	// Relays names relay definitions whose nodes join this selector.
	Relays []string `json:"relays,omitempty"`
}

// Subscription is a single subscription source.
type Subscription struct {
	Name           string   `json:"name"`
	URL            string   `json:"url,omitempty"`
	Path           string   `json:"path,omitempty"`
	Type           string   `json:"type"`
	Enable         bool     `json:"enable"`
	Emoji          *bool    `json:"emoji,omitempty"`
	RemoveKeywords []string `json:"remove_keywords,omitempty"`
	UserAgent      string   `json:"user_agent,omitempty"`
}

// Module is one reusable configuration fragment. Exactly one of File, FromURL
// or Path must be set.
//
// Modules are a flat list: they are not grouped by sing-box section. The section
// a module contributes to is determined by the top-level keys inside the file,
// which is the only thing that actually takes effect.
type Module struct {
	Name    string `json:"name"`
	File    string `json:"file,omitempty"`     // path inside the config repository
	FromURL string `json:"from_url,omitempty"` // externally maintained module
	Path    string `json:"path,omitempty"`     // absolute path on the host, for local development

	// Selectors fills in selector membership for selectors declared in this
	// module's file. The rules apply wherever the module is used.
	Selectors []SelectorRule `json:"selectors,omitempty"`
}

// Source returns a human readable description of where the module comes from.
func (m Module) Source() string {
	switch {
	case m.File != "":
		return "file:" + m.File
	case m.FromURL != "":
		return "url:" + m.FromURL
	case m.Path != "":
		return "path:" + m.Path
	default:
		return "<none>"
	}
}

// ConfigFile describes one generated output file.
type ConfigFile struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Modules []string `json:"modules"`
}

// Schedule controls how often subscriptions are re-fetched. It is unrelated to
// GitHub change detection, which is handled by the webhook and the fallback poll.
type Schedule struct {
	Type  string   `json:"type"`
	Every Duration `json:"every,omitempty"`
}

// LoadConfig parses and validates the repository configuration.
func LoadConfig(data []byte) (*Config, error) {
	var c Config
	if err := strictUnmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config.json: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config.json: %w", err)
	}
	return &c, nil
}

// Validate checks the repository configuration for structural errors.
// It is pure: it does not touch the filesystem or the network.
func (c *Config) Validate() error {
	if c.Nodes == nil {
		return fmt.Errorf("nodes is required")
	}
	if err := c.validateSubscriptions(); err != nil {
		return err
	}
	if err := c.validateRelays(); err != nil {
		return err
	}
	if err := c.validateEmojiOverrides(); err != nil {
		return err
	}
	if err := c.validateModules(); err != nil {
		return err
	}
	if err := c.validateConfigs(); err != nil {
		return err
	}
	if c.UpdateSchedule == nil {
		return fmt.Errorf("update_schedule is required")
	}
	return c.UpdateSchedule.validate()
}

func (c *Config) validateSubscriptions() error {
	seen := make(map[string]int, len(c.Nodes.Subscriptions))
	for i, s := range c.Nodes.Subscriptions {
		where := fmt.Sprintf("nodes.subscriptions[%d]", i)

		if s.Name == "" {
			return fmt.Errorf("%s: name cannot be empty", where)
		}
		if prev, dup := seen[s.Name]; dup {
			return fmt.Errorf("%s: duplicate name %q (already used by nodes.subscriptions[%d])", where, s.Name, prev)
		}
		seen[s.Name] = i

		// Node tags are prefixed with "[name] ", so a bracket in the name would
		// make that marker ambiguous.
		if strings.ContainsAny(s.Name, "[]") {
			return fmt.Errorf("%s (%s): name cannot contain '[' or ']'", where, s.Name)
		}

		hasURL, hasPath := s.URL != "", s.Path != ""
		switch {
		case hasURL && hasPath:
			return fmt.Errorf("%s (%s): set either url or path, not both", where, s.Name)
		case !hasURL && !hasPath:
			return fmt.Errorf("%s (%s): either url or path is required", where, s.Name)
		}
		if hasPath {
			if err := checkRepoRelPath(s.Path); err != nil {
				return fmt.Errorf("%s (%s): path %w", where, s.Name, err)
			}
		}

		valid := []string{SubClash, SubSingBox, SubXray, SubV2Ray}
		if !slices.Contains(valid, strings.ToLower(s.Type)) {
			return fmt.Errorf("%s (%s): unknown type %q (want one of %v)", where, s.Name, s.Type, valid)
		}
	}
	return nil
}

// subscriptionNames returns the set of declared subscription names.
func (c *Config) subscriptionNames() map[string]bool {
	names := make(map[string]bool, len(c.Nodes.Subscriptions))
	for _, s := range c.Nodes.Subscriptions {
		names[s.Name] = true
	}
	return names
}

// RelayNames returns the set of declared relay names.
func (c *Config) RelayNames() map[string]bool {
	names := make(map[string]bool, len(c.Nodes.Relays))
	for _, r := range c.Nodes.Relays {
		names[r.Name] = true
	}
	return names
}

func (c *Config) validateRelays() error {
	subs := c.subscriptionNames()
	seen := make(map[string]int, len(c.Nodes.Relays))

	for i, r := range c.Nodes.Relays {
		where := fmt.Sprintf("nodes.relays[%d]", i)

		if r.Name == "" {
			return fmt.Errorf("%s: name cannot be empty", where)
		}
		if prev, dup := seen[r.Name]; dup {
			return fmt.Errorf("%s: duplicate name %q (already used by nodes.relays[%d])", where, r.Name, prev)
		}
		seen[r.Name] = i

		for _, f := range []struct {
			field string
			sels  []NodeSelector
		}{{"via", r.Via}, {"upstream", r.Upstream}} {
			if len(f.sels) == 0 {
				return fmt.Errorf("%s (%s): %s cannot be empty", where, r.Name, f.field)
			}
			for j, sel := range f.sels {
				if sel.IsEmpty() {
					// A relay template or upstream with no source can only ever
					// produce nothing, which would silently drop the relay.
					return fmt.Errorf("%s (%s): %s[%d] must name at least one subscription in from",
						where, r.Name, f.field, j)
				}
				if err := sel.validate(subs); err != nil {
					return fmt.Errorf("%s (%s): %s[%d]: %w", where, r.Name, f.field, j, err)
				}
			}
		}
	}
	return nil
}

// validate checks that a selector references only declared subscriptions.
func (s NodeSelector) validate(subs map[string]bool) error {
	for _, name := range s.From {
		if !subs[name] {
			return fmt.Errorf("unknown subscription %q in from", name)
		}
	}
	return nil
}

func (c *Config) validateEmojiOverrides() error {
	for i, r := range c.Nodes.EmojiOverrides {
		where := fmt.Sprintf("nodes.emoji_overrides[%d]", i)
		if r.Emoji == "" {
			return fmt.Errorf("%s: emoji cannot be empty", where)
		}
		if len(r.Keywords) == 0 {
			return fmt.Errorf("%s (%s): keywords cannot be empty", where, r.Emoji)
		}
		for j, kw := range r.Keywords {
			if kw == "" {
				return fmt.Errorf("%s (%s): keywords[%d] cannot be empty", where, r.Emoji, j)
			}
		}
	}
	return nil
}

func (c *Config) validateModules() error {
	if len(c.Modules) == 0 {
		return fmt.Errorf("modules cannot be empty")
	}
	seen := make(map[string]int, len(c.Modules))
	for i, m := range c.Modules {
		where := fmt.Sprintf("modules[%d]", i)

		if m.Name == "" {
			return fmt.Errorf("%s: name cannot be empty", where)
		}
		if prev, dup := seen[m.Name]; dup {
			return fmt.Errorf("%s: duplicate name %q (already used by modules[%d])", where, m.Name, prev)
		}
		seen[m.Name] = i

		n := 0
		for _, set := range []bool{m.File != "", m.FromURL != "", m.Path != ""} {
			if set {
				n++
			}
		}
		switch {
		case n == 0:
			return fmt.Errorf("%s (%s): one of file, from_url or path is required", where, m.Name)
		case n > 1:
			return fmt.Errorf("%s (%s): set exactly one of file, from_url or path", where, m.Name)
		}
		if m.File != "" {
			if err := checkRepoRelPath(m.File); err != nil {
				return fmt.Errorf("%s (%s): file %w", where, m.Name, err)
			}
		}
		if err := c.validateSelectors(m, where); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateSelectors(m Module, where string) error {
	subs := c.subscriptionNames()
	relays := c.RelayNames()
	seen := make(map[string]int, len(m.Selectors))

	for i, rule := range m.Selectors {
		at := fmt.Sprintf("%s (%s): selectors[%d]", where, m.Name, i)

		if rule.Tag == "" {
			return fmt.Errorf("%s: tag cannot be empty", at)
		}
		if prev, dup := seen[rule.Tag]; dup {
			return fmt.Errorf("%s: duplicate tag %q (already used by selectors[%d])", at, rule.Tag, prev)
		}
		seen[rule.Tag] = i

		if err := rule.NodeSelector.validate(subs); err != nil {
			return fmt.Errorf("%s (%s): %w", at, rule.Tag, err)
		}
		for _, name := range rule.Relays {
			if !relays[name] {
				return fmt.Errorf("%s (%s): unknown relay %q", at, rule.Tag, name)
			}
		}
		if rule.NodeSelector.IsEmpty() && len(rule.Relays) == 0 {
			// Such a rule contributes nothing; the selector would end up with
			// only whatever its module file already listed.
			return fmt.Errorf("%s (%s): selects nothing; set from, relays, or remove the rule", at, rule.Tag)
		}
	}
	return nil
}

func (c *Config) validateConfigs() error {
	if len(c.Configs) == 0 {
		return fmt.Errorf("configs cannot be empty")
	}
	known := make(map[string]bool, len(c.Modules))
	for _, m := range c.Modules {
		known[m.Name] = true
	}

	seenName := make(map[string]int, len(c.Configs))
	for i, cf := range c.Configs {
		where := fmt.Sprintf("configs[%d]", i)

		if cf.Name == "" {
			return fmt.Errorf("%s: name cannot be empty", where)
		}
		if prev, dup := seenName[cf.Name]; dup {
			return fmt.Errorf("%s: duplicate name %q (already used by configs[%d])", where, cf.Name, prev)
		}
		seenName[cf.Name] = i

		if cf.Path == "" {
			return fmt.Errorf("%s (%s): path cannot be empty", where, cf.Name)
		}
		if len(cf.Modules) == 0 {
			return fmt.Errorf("%s (%s): modules cannot be empty", where, cf.Name)
		}
		seenMod := make(map[string]bool, len(cf.Modules))
		for _, name := range cf.Modules {
			if !known[name] {
				return fmt.Errorf("%s (%s): unknown module %q", where, cf.Name, name)
			}
			if seenMod[name] {
				return fmt.Errorf("%s (%s): module %q listed more than once", where, cf.Name, name)
			}
			seenMod[name] = true
		}
	}
	return nil
}

func (s *Schedule) validate() error {
	switch strings.ToLower(s.Type) {
	case ScheduleInterval:
		if s.Every.IsZero() {
			return fmt.Errorf("update_schedule.every is required when type is %q", ScheduleInterval)
		}
		if s.Every.Duration() < 0 {
			return fmt.Errorf("update_schedule.every must be positive, got %s", s.Every)
		}
	case ScheduleHourly:
		if !s.Every.IsZero() {
			return fmt.Errorf("update_schedule.every is not used when type is %q", ScheduleHourly)
		}
	case "":
		return fmt.Errorf("update_schedule.type is required (%q or %q)", ScheduleInterval, ScheduleHourly)
	default:
		return fmt.Errorf("unknown update_schedule.type %q (want %q or %q)", s.Type, ScheduleInterval, ScheduleHourly)
	}
	return nil
}

// checkRepoRelPath rejects paths that are absolute or escape the snapshot root.
// The returned error is phrased to read after a "field " prefix.
func checkRepoRelPath(p string) error {
	if path.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return fmt.Errorf("%q must be relative to the repository root", p)
	}
	if strings.Contains(p, `\`) {
		return fmt.Errorf("%q must use forward slashes", p)
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%q must not escape the repository root", p)
	}
	return nil
}
