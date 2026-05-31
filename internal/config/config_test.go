package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"node-box/internal/config"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeConfigJSON(t *testing.T, dir string, name string, cfg map[string]any) string {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func minimalValidConfig() map[string]any {
	return map[string]any{
		"nodes": map[string]any{
			"subscriptions": []any{
				map[string]any{
					"name":   "test-sub",
					"url":    "https://example.com/sub",
					"type":   "clash",
					"enable": true,
				},
			},
		},
		"update_schedule": map[string]any{
			"type":     "interval",
			"interval": 6,
		},
	}
}

// ---------------------------------------------------------------------------
// Load
// ---------------------------------------------------------------------------

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigJSON(t, dir, "config.json", minimalValidConfig())

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Nodes == nil {
		t.Error("expected nodes to be populated")
	}
	if cfg.UpdateSchedule == nil {
		t.Error("expected update_schedule to be populated")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load("/nonexistent/config.json")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json {{{"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoad_WithProxy(t *testing.T) {
	dir := t.TempDir()
	cfg := minimalValidConfig()
	cfg["proxy"] = map[string]any{
		"type": "http",
		"host": "127.0.0.1",
		"port": 8080,
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.Proxy == nil {
		t.Error("expected proxy to be populated")
	}
	if loaded.Proxy.Host != "127.0.0.1" {
		t.Errorf("expected proxy host 127.0.0.1, got %s", loaded.Proxy.Host)
	}
}

// ---------------------------------------------------------------------------
// Config.Validate
// ---------------------------------------------------------------------------

func TestValidate_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigJSON(t, dir, "config.json", minimalValidConfig())

	cfg, _ := config.Load(path)
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestValidate_MissingNodes(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"update_schedule": map[string]any{"type": "interval", "interval": 6},
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error for missing nodes")
	}
}

func TestValidate_MissingUpdateSchedule(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"nodes": map[string]any{
			"subscriptions": []any{},
		},
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error for missing update_schedule")
	}
}

func TestValidate_InvalidSubscriptionType(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"nodes": map[string]any{
			"subscriptions": []any{
				map[string]any{
					"name":   "bad-sub",
					"url":    "https://example.com/sub",
					"type":   "invalid-type",
					"enable": true,
				},
			},
		},
		"update_schedule": map[string]any{"type": "interval", "interval": 6},
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error for invalid subscription type")
	}
}

func TestValidate_SubscriptionMissingName(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"nodes": map[string]any{
			"subscriptions": []any{
				map[string]any{
					"name":   "",
					"url":    "https://example.com/sub",
					"type":   "clash",
					"enable": true,
				},
			},
		},
		"update_schedule": map[string]any{"type": "interval", "interval": 6},
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error for subscription with empty name")
	}
}

func TestValidate_SubscriptionBothURLAndPath(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"nodes": map[string]any{
			"subscriptions": []any{
				map[string]any{
					"name":   "both",
					"url":    "https://example.com/sub",
					"path":   "/local/path",
					"type":   "clash",
					"enable": true,
				},
			},
		},
		"update_schedule": map[string]any{"type": "interval", "interval": 6},
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error when both URL and Path are specified")
	}
}

func TestValidate_SubscriptionNeitherURLNorPath(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"nodes": map[string]any{
			"subscriptions": []any{
				map[string]any{
					"name":   "neither",
					"type":   "clash",
					"enable": true,
				},
			},
		},
		"update_schedule": map[string]any{"type": "interval", "interval": 6},
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error when neither URL nor Path is specified")
	}
}

func TestValidate_AllSubscriptionTypes(t *testing.T) {
	validTypes := []string{"clash", "singbox", "relay", "xray", "v2ray"}
	for _, typ := range validTypes {
		t.Run(typ, func(t *testing.T) {
			dir := t.TempDir()
			cfg := map[string]any{
				"nodes": map[string]any{
					"subscriptions": []any{
						map[string]any{
							"name":   "test",
							"url":    "https://example.com/sub",
							"type":   typ,
							"enable": true,
						},
					},
				},
				"update_schedule": map[string]any{"type": "interval", "interval": 6},
			}
			path := writeConfigJSON(t, dir, "config.json", cfg)
			loaded, _ := config.Load(path)
			if err := loaded.Validate(); err != nil {
				t.Errorf("type %q should be valid, got error: %v", typ, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Proxy validation
// ---------------------------------------------------------------------------

func TestValidate_ProxyEmptyHost(t *testing.T) {
	dir := t.TempDir()
	cfg := minimalValidConfig()
	cfg["proxy"] = map[string]any{
		"type": "http",
		"host": "",
		"port": 8080,
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error for proxy with empty host")
	}
}

func TestValidate_ProxyInvalidPort(t *testing.T) {
	dir := t.TempDir()
	cfg := minimalValidConfig()
	cfg["proxy"] = map[string]any{
		"type": "http",
		"host": "127.0.0.1",
		"port": 99999,
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error for proxy with invalid port")
	}
}

func TestValidate_ProxyInvalidType(t *testing.T) {
	dir := t.TempDir()
	cfg := minimalValidConfig()
	cfg["proxy"] = map[string]any{
		"type": "ftp",
		"host": "127.0.0.1",
		"port": 21,
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error for proxy with invalid type")
	}
}

// ---------------------------------------------------------------------------
// Schedule validation
// ---------------------------------------------------------------------------

func TestValidate_ScheduleInterval_ZeroInterval(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"nodes": map[string]any{
			"subscriptions": []any{
				map[string]any{"name": "s", "url": "https://example.com", "type": "clash", "enable": true},
			},
		},
		"update_schedule": map[string]any{
			"type":     "interval",
			"interval": 0,
		},
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error for interval=0 with type=interval")
	}
}

func TestValidate_ScheduleHourly_Valid(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"nodes": map[string]any{
			"subscriptions": []any{
				map[string]any{"name": "s", "url": "https://example.com", "type": "clash", "enable": true},
			},
		},
		"update_schedule": map[string]any{
			"type": "hourly",
		},
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err != nil {
		t.Errorf("hourly schedule should be valid, got: %v", err)
	}
}

func TestValidate_ScheduleInvalidType(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"nodes": map[string]any{
			"subscriptions": []any{
				map[string]any{"name": "s", "url": "https://example.com", "type": "clash", "enable": true},
			},
		},
		"update_schedule": map[string]any{
			"type": "cron",
		},
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error for invalid schedule type")
	}
}

// ---------------------------------------------------------------------------
// GetConfigPath
// ---------------------------------------------------------------------------

func TestGetConfigPath_ProvidedPath(t *testing.T) {
	result := config.GetConfigPath("/custom/path.json", "default.json")
	if result != "/custom/path.json" {
		t.Errorf("expected /custom/path.json, got %s", result)
	}
}

func TestGetConfigPath_DefaultPath(t *testing.T) {
	// Ensure env var is not set
	os.Unsetenv(config.ConfigPathEnvVar)
	result := config.GetConfigPath("", "default.json")
	if result != "default.json" {
		t.Errorf("expected default.json, got %s", result)
	}
}

func TestGetConfigPath_EnvVar(t *testing.T) {
	os.Setenv(config.ConfigPathEnvVar, "/env/path.json")
	defer os.Unsetenv(config.ConfigPathEnvVar)

	result := config.GetConfigPath("", "default.json")
	if result != "/env/path.json" {
		t.Errorf("expected /env/path.json, got %s", result)
	}
}

func TestGetConfigPath_ProvidedOverridesEnv(t *testing.T) {
	os.Setenv(config.ConfigPathEnvVar, "/env/path.json")
	defer os.Unsetenv(config.ConfigPathEnvVar)

	result := config.GetConfigPath("/explicit/path.json", "default.json")
	if result != "/explicit/path.json" {
		t.Errorf("expected /explicit/path.json, got %s", result)
	}
}

// ---------------------------------------------------------------------------
// ModulesConfig helpers
// ---------------------------------------------------------------------------

func TestModulesConfig_ModuleEntries_AllTypes(t *testing.T) {
	m := &config.ModulesConfig{
		Log:      []config.Module{{Name: "log-mod", Path: "/log"}},
		DNS:      []config.Module{{Name: "dns-mod", Path: "/dns"}},
		Inbounds: []config.Module{{Name: "in-mod", Path: "/in"}},
	}
	entries := m.ModuleEntries()
	if len(entries) == 0 {
		t.Error("expected non-empty module entries")
	}
	// Verify log and dns are present
	found := make(map[string]bool)
	for _, e := range entries {
		found[e.Type] = true
	}
	if !found["log"] {
		t.Error("expected log entry")
	}
	if !found["dns"] {
		t.Error("expected dns entry")
	}
}

func TestModulesConfig_AllModuleNames(t *testing.T) {
	m := &config.ModulesConfig{
		Log: []config.Module{{Name: "log-mod", Path: "/log"}},
		DNS: []config.Module{{Name: "dns-mod", Path: "/dns"}},
	}
	names := m.AllModuleNames()
	if !names["log-mod"] {
		t.Error("expected log-mod in names")
	}
	if !names["dns-mod"] {
		t.Error("expected dns-mod in names")
	}
}

func TestModulesConfig_ModulesByType(t *testing.T) {
	m := &config.ModulesConfig{
		DNS: []config.Module{{Name: "dns-mod", Path: "/dns"}},
	}
	mods := m.ModulesByType("dns")
	if len(mods) != 1 || mods[0].Name != "dns-mod" {
		t.Errorf("expected dns-mod, got %v", mods)
	}
	// Unknown type returns nil
	if m.ModulesByType("unknown") != nil {
		t.Error("expected nil for unknown type")
	}
}

// ---------------------------------------------------------------------------
// Module validation
// ---------------------------------------------------------------------------

func TestValidate_Module_BothPathAndURL(t *testing.T) {
	dir := t.TempDir()
	cfg := minimalValidConfig()
	cfg["modules"] = map[string]any{
		"dns": []any{
			map[string]any{
				"name":     "dns-mod",
				"path":     "/local/dns.json",
				"from_url": "https://example.com/dns.json",
			},
		},
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error when module has both path and from_url")
	}
}

func TestValidate_Module_NeitherPathNorURL(t *testing.T) {
	dir := t.TempDir()
	cfg := minimalValidConfig()
	cfg["modules"] = map[string]any{
		"dns": []any{
			map[string]any{
				"name": "dns-mod",
			},
		},
	}
	path := writeConfigJSON(t, dir, "config.json", cfg)

	loaded, _ := config.Load(path)
	if err := loaded.Validate(); err == nil {
		t.Error("expected error when module has neither path nor from_url")
	}
}
