package model

import (
	"path/filepath"
	"strings"
	"testing"
)

// validConfig is a minimal configuration that passes validation. Tests mutate
// a copy of it to exercise one rule at a time.
func validConfig() *Config {
	return &Config{
		Nodes: &NodesConfig{
			Subscriptions: []Subscription{
				{Name: "airport-a", URL: "https://example.com/sub", Type: SubClash, Enable: true},
			},
		},
		Modules: []Module{
			{Name: "log", File: "modules/log.json"},
			{Name: "dns", File: "modules/dns.json"},
		},
		Configs: []ConfigFile{
			{Name: "main", Path: "main.json", Modules: []string{"log", "dns"}},
		},
		UpdateSchedule: &Schedule{Type: ScheduleInterval, Every: Duration(6e9)},
	}
}

func wantErr(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want an error mentioning %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q should mention %q", err, substr)
	}
}

func TestConfig_Valid(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestConfig_DuplicateSubscriptionName(t *testing.T) {
	c := validConfig()
	c.Nodes.Subscriptions = append(c.Nodes.Subscriptions, Subscription{
		Name: "airport-a", URL: "https://other.example.com", Type: SubSingBox,
	})
	// Duplicate names silently overwrote each other in the cache before and
	// produced colliding "[name]" tag prefixes.
	wantErr(t, c.Validate(), "duplicate name")
}

func TestConfig_SubscriptionNameWithBrackets(t *testing.T) {
	c := validConfig()
	c.Nodes.Subscriptions[0].Name = "air[port]"
	wantErr(t, c.Validate(), "cannot contain")
}

func TestConfig_SubscriptionSourceExclusivity(t *testing.T) {
	t.Run("both", func(t *testing.T) {
		c := validConfig()
		c.Nodes.Subscriptions[0].Path = "subs/a.yaml"
		wantErr(t, c.Validate(), "not both")
	})
	t.Run("neither", func(t *testing.T) {
		c := validConfig()
		c.Nodes.Subscriptions[0].URL = ""
		wantErr(t, c.Validate(), "required")
	})
}

func TestConfig_UnknownSubscriptionType(t *testing.T) {
	c := validConfig()
	c.Nodes.Subscriptions[0].Type = "surge"
	wantErr(t, c.Validate(), "unknown type")
}

func TestConfig_ModuleSourceExclusivity(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		c := validConfig()
		c.Modules[0] = Module{Name: "log"}
		wantErr(t, c.Validate(), "one of file, from_url or path")
	})
	t.Run("two", func(t *testing.T) {
		c := validConfig()
		c.Modules[0].FromURL = "https://example.com/log.json"
		wantErr(t, c.Validate(), "exactly one")
	})
}

func TestConfig_ModuleFileMustStayInRepo(t *testing.T) {
	for name, p := range map[string]string{
		"absolute": "/etc/passwd",
		"escaping": "../../secrets.json",
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig()
			c.Modules[0].File = p
			if err := c.Validate(); err == nil {
				t.Fatalf("path %q should be rejected", p)
			}
		})
	}
}

func TestConfig_UnknownModuleReference(t *testing.T) {
	c := validConfig()
	c.Configs[0].Modules = []string{"log", "typo"}
	wantErr(t, c.Validate(), "unknown module")
}

func TestConfig_DuplicateModuleInOneOutput(t *testing.T) {
	c := validConfig()
	c.Configs[0].Modules = []string{"log", "log"}
	// Listing a module twice would always trip the key-conflict check in the
	// builder; catching it here names the real mistake.
	wantErr(t, c.Validate(), "more than once")
}

func TestConfig_Schedule(t *testing.T) {
	t.Run("interval needs every", func(t *testing.T) {
		c := validConfig()
		c.UpdateSchedule = &Schedule{Type: ScheduleInterval}
		wantErr(t, c.Validate(), "every is required")
	})
	t.Run("hourly rejects every", func(t *testing.T) {
		c := validConfig()
		c.UpdateSchedule = &Schedule{Type: ScheduleHourly, Every: Duration(6e9)}
		wantErr(t, c.Validate(), "not used")
	})
	t.Run("hourly alone is fine", func(t *testing.T) {
		c := validConfig()
		c.UpdateSchedule = &Schedule{Type: ScheduleHourly}
		if err := c.Validate(); err != nil {
			t.Fatalf("hourly schedule rejected: %v", err)
		}
	})
}

func TestLoadConfig_RejectsUnknownFields(t *testing.T) {
	// A typo in a key name used to be silently ignored, so the setting simply
	// never took effect.
	data := []byte(`{
      "nodes": {"subscriptions": []},
      "modules": [{"name":"log","file":"modules/log.json"}],
      "configs": [{"name":"main","path":"main.json","modules":["log"]}],
      "update_schedule": {"type":"interval","evrey":"6h"}
    }`)
	_, err := LoadConfig(data)
	wantErr(t, err, "evrey")
}

func TestLoadConfig_DurationIsAString(t *testing.T) {
	data := []byte(`{
      "nodes": {"subscriptions": []},
      "modules": [{"name":"log","file":"modules/log.json"}],
      "configs": [{"name":"main","path":"main.json","modules":["log"]}],
      "update_schedule": {"type":"interval","every":6}
    }`)
	wantErr(t, mustErr(LoadConfig(data)), "duration must be a string")
}

func mustErr(_ *Config, err error) error { return err }

func TestResolveOutputs(t *testing.T) {
	// t.TempDir returns a path that is absolute on every OS, which matters
	// because filepath.IsAbs is platform specific.
	root := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "sing-box", "config.json")
	b := &Bootstrap{Root: root}

	c := validConfig()
	c.Configs = []ConfigFile{
		{Name: "main", Path: "main.json", Modules: []string{"log"}},
		{Name: "nested", Path: filepath.FromSlash("sub/gaming.json"), Modules: []string{"log"}},
		{Name: "absolute", Path: elsewhere, Modules: []string{"log"}},
	}

	outs, err := c.ResolveOutputs(b)
	if err != nil {
		t.Fatalf("ResolveOutputs: %v", err)
	}

	want := []string{
		filepath.Join(root, "out", "main.json"),
		filepath.Join(root, "out", "sub", "gaming.json"),
		elsewhere,
	}
	for i, w := range want {
		if outs[i].Path != w {
			t.Errorf("output %d = %q, want %q", i, outs[i].Path, w)
		}
	}
}

func TestResolveOutputs_CustomDir(t *testing.T) {
	root := t.TempDir()
	b := &Bootstrap{Root: root}

	c := validConfig()
	c.Output = &OutputConfig{Dir: "generated"}
	c.Configs = []ConfigFile{{Name: "main", Path: "main.json", Modules: []string{"log"}}}

	outs, err := c.ResolveOutputs(b)
	if err != nil {
		t.Fatalf("ResolveOutputs: %v", err)
	}
	if want := filepath.Join(root, "generated", "main.json"); outs[0].Path != want {
		t.Errorf("path = %q, want %q", outs[0].Path, want)
	}
}

func TestResolveOutputs_RejectsCollision(t *testing.T) {
	b := &Bootstrap{Root: t.TempDir()}
	c := validConfig()
	c.Configs = []ConfigFile{
		{Name: "a", Path: "main.json", Modules: []string{"log"}},
		{Name: "b", Path: filepath.FromSlash("./main.json"), Modules: []string{"log"}},
	}
	// Two outputs writing the same file meant the later one silently won.
	_, err := c.ResolveOutputs(b)
	wantErr(t, err, "both resolve to")
}

func TestResolveOutputs_RejectsWritingIntoSnapshots(t *testing.T) {
	root := t.TempDir()
	b := &Bootstrap{Root: root}
	c := validConfig()
	c.Configs = []ConfigFile{{
		Name:    "bad",
		Path:    filepath.Join(root, "snapshots", "abc", "main.json"),
		Modules: []string{"log"},
	}}
	// Snapshots are immutable inputs; writing into them breaks the whole model.
	_, err := c.ResolveOutputs(b)
	wantErr(t, err, "read-only snapshot")
}
