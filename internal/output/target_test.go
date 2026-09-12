package output

import (
	"path/filepath"
	"strings"
	"testing"

	"node-box/internal/model"
)

// validConfig is the smallest configuration that passes validation, which is all
// destination resolution needs.
func validConfig() *model.Config {
	return &model.Config{
		Nodes: &model.NodesConfig{
			Subscriptions: []model.Subscription{
				{Name: "sub", URL: "https://example.com/s", Type: model.SubClash, Enable: true},
			},
		},
		Modules:        []model.Module{{Name: "mod", File: "modules/mod.json"}},
		Configs:        []model.ConfigFile{{Name: "main", Path: "main.json", Modules: []string{"mod"}}},
		UpdateSchedule: &model.Schedule{Type: model.ScheduleHourly},
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

func TestResolve(t *testing.T) {
	// t.TempDir returns a path that is absolute on every OS, which matters
	// because filepath.IsAbs is platform specific.
	root := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "sing-box", "config.json")
	b := &model.Bootstrap{Root: root}

	c := validConfig()
	c.Configs = []model.ConfigFile{
		{Name: "main", Path: "main.json", Modules: []string{"log"}},
		{Name: "nested", Path: filepath.FromSlash("sub/gaming.json"), Modules: []string{"log"}},
		{Name: "absolute", Path: elsewhere, Modules: []string{"log"}},
	}

	outs, err := Resolve(c, b)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
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

func TestResolve_CustomDir(t *testing.T) {
	root := t.TempDir()
	b := &model.Bootstrap{Root: root}

	c := validConfig()
	c.Output = &model.OutputConfig{Dir: "generated"}
	c.Configs = []model.ConfigFile{{Name: "main", Path: "main.json", Modules: []string{"log"}}}

	outs, err := Resolve(c, b)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := filepath.Join(root, "generated", "main.json"); outs[0].Path != want {
		t.Errorf("path = %q, want %q", outs[0].Path, want)
	}
}

func TestResolve_RejectsCollision(t *testing.T) {
	b := &model.Bootstrap{Root: t.TempDir()}
	c := validConfig()
	c.Configs = []model.ConfigFile{
		{Name: "a", Path: "main.json", Modules: []string{"log"}},
		{Name: "b", Path: filepath.FromSlash("./main.json"), Modules: []string{"log"}},
	}
	// Two outputs writing the same file meant the later one silently won.
	_, err := Resolve(c, b)
	wantErr(t, err, "both resolve to")
}

func TestResolve_RejectsWritingIntoSnapshots(t *testing.T) {
	root := t.TempDir()
	b := &model.Bootstrap{Root: root}
	c := validConfig()
	c.Configs = []model.ConfigFile{{
		Name:    "bad",
		Path:    filepath.Join(root, "snapshots", "abc", "main.json"),
		Modules: []string{"log"},
	}}
	// Snapshots are immutable inputs; writing into them breaks the whole model.
	_, err := Resolve(c, b)
	wantErr(t, err, "read-only snapshot")
}

// --- relays and selector rules ---
