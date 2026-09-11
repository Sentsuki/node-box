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
	SubRelay   = "relay" // TODO(outbounds): relay handling is not designed yet.
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

// NodesConfig holds subscription sources and global filtering.
type NodesConfig struct {
	Subscriptions   []Subscription `json:"subscriptions"`
	ExcludeKeywords []string       `json:"exclude_keywords,omitempty"`

	// TODO(outbounds): relay node generation rules, pending redesign.
	RelayNodes []RelayRule `json:"relay_nodes,omitempty"`
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

// RelayRule is a relay node generation rule.
//
// TODO(outbounds): pending redesign along with outbounds/endpoints.
type RelayRule struct {
	Tag      string   `json:"tag"`
	Upstream []string `json:"upstream"`
}

// Selector describes how subscription nodes are inserted into a selector.
//
// TODO(outbounds): pending redesign along with outbounds/endpoints.
type Selector struct {
	InsertMarker      string   `json:"insert_marker"`
	IncludeNodes      []string `json:"include_nodes,omitempty"`
	ExcludeNodes      []string `json:"exclude_nodes,omitempty"`
	IncludeRelayNodes []string `json:"include_relay_nodes,omitempty"`
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

	// TODO(outbounds): node injection rules, pending redesign.
	Selectors     []Selector `json:"selectors,omitempty"`
	Subscriptions []string   `json:"subscriptions,omitempty"`
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

	// TODO(outbounds): output-level node filtering, pending redesign.
	NoNeedNodes []string `json:"no_need_nodes,omitempty"`
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

		valid := []string{SubClash, SubSingBox, SubXray, SubV2Ray, SubRelay}
		if !slices.Contains(valid, strings.ToLower(s.Type)) {
			return fmt.Errorf("%s (%s): unknown type %q (want one of %v)", where, s.Name, s.Type, valid)
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
