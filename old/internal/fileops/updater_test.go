package fileops_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"node-box/internal/fileops"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeConfigFile writes a SingBox-like config JSON to a temp file and returns the path.
func writeConfigFile(t *testing.T, dir string, name string, cfg map[string]any) string {
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

// readConfigFile reads and parses a JSON config file.
func readConfigFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return cfg
}

// getOutbounds extracts the outbounds array from a config map.
func getOutbounds(t *testing.T, cfg map[string]any) []any {
	t.Helper()
	raw, ok := cfg["outbounds"]
	if !ok {
		t.Fatal("missing outbounds field")
	}
	arr, ok := raw.([]any)
	if !ok {
		t.Fatalf("outbounds is not an array: %T", raw)
	}
	return arr
}

// buildBaseConfig creates a minimal SingBox config with a selector marker.
func buildBaseConfig(markerTag string) map[string]any {
	return map[string]any{
		"outbounds": []any{
			map[string]any{
				"type":      "selector",
				"tag":       markerTag,
				"outbounds": []any{"direct"},
			},
			map[string]any{"type": "direct", "tag": "direct"},
		},
	}
}

// ---------------------------------------------------------------------------
// UpdateConfigFile
// ---------------------------------------------------------------------------

func TestUpdater_UpdateConfigFile_InsertsNodes(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, "config.json", buildBaseConfig("🎮 游戏节点"))

	nodes := []map[string]any{
		{"type": "vmess", "tag": "[sub] 美国 01", "server": "1.2.3.4", "server_port": 443},
		{"type": "vmess", "tag": "[sub] 日本 02", "server": "5.6.7.8", "server_port": 443},
	}

	u := fileops.NewUpdater("🎮 游戏节点")
	err := u.UpdateConfigFile(path, nodes, []string{"sub"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := readConfigFile(t, path)
	outbounds := getOutbounds(t, cfg)

	// Should have: selector + direct + 2 new nodes = 4
	if len(outbounds) != 4 {
		t.Errorf("expected 4 outbounds, got %d", len(outbounds))
	}

	// Verify nodes were inserted
	tags := make(map[string]bool)
	for _, ob := range outbounds {
		if m, ok := ob.(map[string]any); ok {
			if tag, ok := m["tag"].(string); ok {
				tags[tag] = true
			}
		}
	}
	if !tags["[sub] 美国 01"] {
		t.Error("expected [sub] 美国 01 in outbounds")
	}
	if !tags["[sub] 日本 02"] {
		t.Error("expected [sub] 日本 02 in outbounds")
	}
}

func TestUpdater_UpdateConfigFile_UpdatesSelectorOutbounds(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, "config.json", buildBaseConfig("my-selector"))

	nodes := []map[string]any{
		{"type": "vmess", "tag": "[sub] 节点 A", "server": "1.2.3.4", "server_port": 443},
		{"type": "vmess", "tag": "[sub] 节点 B", "server": "5.6.7.8", "server_port": 443},
	}

	u := fileops.NewUpdater("my-selector")
	err := u.UpdateConfigFile(path, nodes, []string{"sub"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := readConfigFile(t, path)
	outbounds := getOutbounds(t, cfg)

	// Find the selector and check its outbounds
	for _, ob := range outbounds {
		m, ok := ob.(map[string]any)
		if !ok {
			continue
		}
		if m["tag"] == "my-selector" {
			selectorOutbounds, ok := m["outbounds"].([]any)
			if !ok {
				t.Fatal("selector outbounds is not an array")
			}
			found := make(map[string]bool)
			for _, s := range selectorOutbounds {
				if str, ok := s.(string); ok {
					found[str] = true
				}
			}
			if !found["[sub] 节点 A"] {
				t.Error("selector should contain [sub] 节点 A")
			}
			if !found["[sub] 节点 B"] {
				t.Error("selector should contain [sub] 节点 B")
			}
			return
		}
	}
	t.Error("selector not found in outbounds")
}

func TestUpdater_UpdateConfigFile_CleansOldNodes(t *testing.T) {
	dir := t.TempDir()
	// Config already has old subscription nodes
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{
				"type":      "selector",
				"tag":       "my-selector",
				"outbounds": []any{"direct", "[oldsub] 旧节点"},
			},
			map[string]any{"type": "vmess", "tag": "[oldsub] 旧节点", "server": "old.server", "server_port": 443},
			map[string]any{"type": "direct", "tag": "direct"},
		},
	}
	path := writeConfigFile(t, dir, "config.json", cfg)

	newNodes := []map[string]any{
		{"type": "vmess", "tag": "[newsub] 新节点", "server": "new.server", "server_port": 443},
	}

	u := fileops.NewUpdater("my-selector")
	err := u.UpdateConfigFile(path, newNodes, []string{"oldsub", "newsub"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := readConfigFile(t, path)
	outbounds := getOutbounds(t, result)

	// Old node should be gone
	for _, ob := range outbounds {
		if m, ok := ob.(map[string]any); ok {
			if m["tag"] == "[oldsub] 旧节点" {
				t.Error("old subscription node should have been removed")
			}
		}
	}
}

func TestUpdater_UpdateConfigFile_MarkerNotFound(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"type": "direct", "tag": "direct"},
		},
	}
	path := writeConfigFile(t, dir, "config.json", cfg)

	u := fileops.NewUpdater("nonexistent-marker")
	err := u.UpdateConfigFile(path, nil, nil, nil, nil)
	if err == nil {
		t.Error("expected error when marker not found")
	}
}

func TestUpdater_UpdateConfigFile_InvalidMarkerType(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"type": "vmess", "tag": "not-a-selector", "server": "1.2.3.4", "server_port": 443},
		},
	}
	path := writeConfigFile(t, dir, "config.json", cfg)

	u := fileops.NewUpdater("not-a-selector")
	err := u.UpdateConfigFile(path, nil, nil, nil, nil)
	if err == nil {
		t.Error("expected error when marker is not selector/urltest type")
	}
}

func TestUpdater_UpdateConfigFile_MissingOutbounds(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{"log": map[string]any{"level": "info"}}
	path := writeConfigFile(t, dir, "config.json", cfg)

	u := fileops.NewUpdater("my-selector")
	err := u.UpdateConfigFile(path, nil, nil, nil, nil)
	if err == nil {
		t.Error("expected error when outbounds field is missing")
	}
}

func TestUpdater_UpdateConfigFile_FileNotFound(t *testing.T) {
	u := fileops.NewUpdater("my-selector")
	err := u.UpdateConfigFile("/nonexistent/config.json", nil, nil, nil, nil)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// ---------------------------------------------------------------------------
// UpdateConfigFile – include/exclude keywords for selector
// ---------------------------------------------------------------------------

func TestUpdater_UpdateConfigFile_IncludeKeywords(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, "config.json", buildBaseConfig("my-selector"))

	nodes := []map[string]any{
		{"type": "vmess", "tag": "[sub] 美国 01", "server": "1.2.3.4", "server_port": 443},
		{"type": "vmess", "tag": "[sub] 日本 02", "server": "5.6.7.8", "server_port": 443},
		{"type": "vmess", "tag": "[sub] 香港 03", "server": "9.10.11.12", "server_port": 443},
	}

	u := fileops.NewUpdater("my-selector")
	// Only include 美国 nodes in selector
	err := u.UpdateConfigFile(path, nodes, []string{"sub"}, []string{"美国"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := readConfigFile(t, path)
	outbounds := getOutbounds(t, cfg)

	for _, ob := range outbounds {
		m, ok := ob.(map[string]any)
		if !ok || m["tag"] != "my-selector" {
			continue
		}
		selectorOutbounds := m["outbounds"].([]any)
		for _, s := range selectorOutbounds {
			str, ok := s.(string)
			if !ok {
				continue
			}
			if str == "[sub] 日本 02" || str == "[sub] 香港 03" {
				t.Errorf("non-included node %q should not be in selector", str)
			}
		}
		return
	}
}

func TestUpdater_UpdateConfigFile_ExcludeKeywords(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, "config.json", buildBaseConfig("my-selector"))

	nodes := []map[string]any{
		{"type": "vmess", "tag": "[sub] 美国 01", "server": "1.2.3.4", "server_port": 443},
		{"type": "vmess", "tag": "[sub] 日本 02", "server": "5.6.7.8", "server_port": 443},
	}

	u := fileops.NewUpdater("my-selector")
	// Exclude 日本 from selector
	err := u.UpdateConfigFile(path, nodes, []string{"sub"}, nil, []string{"日本"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := readConfigFile(t, path)
	outbounds := getOutbounds(t, cfg)

	for _, ob := range outbounds {
		m, ok := ob.(map[string]any)
		if !ok || m["tag"] != "my-selector" {
			continue
		}
		selectorOutbounds := m["outbounds"].([]any)
		for _, s := range selectorOutbounds {
			if s == "[sub] 日本 02" {
				t.Error("excluded node [sub] 日本 02 should not be in selector")
			}
		}
		return
	}
}

// ---------------------------------------------------------------------------
// CleanAllSubscriptionArtifacts
// ---------------------------------------------------------------------------

func TestUpdater_CleanAllSubscriptionArtifacts(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{
				"type":      "selector",
				"tag":       "my-selector",
				"outbounds": []any{"direct", "[sub1] 节点 A", "[sub2] 节点 B"},
			},
			map[string]any{"type": "vmess", "tag": "[sub1] 节点 A", "server": "1.2.3.4", "server_port": 443},
			map[string]any{"type": "vmess", "tag": "[sub2] 节点 B", "server": "5.6.7.8", "server_port": 443},
			map[string]any{"type": "direct", "tag": "direct"},
		},
	}
	path := writeConfigFile(t, dir, "config.json", cfg)

	u := fileops.NewUpdater("my-selector")
	err := u.CleanAllSubscriptionArtifacts(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := readConfigFile(t, path)
	outbounds := getOutbounds(t, result)

	for _, ob := range outbounds {
		m, ok := ob.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := m["tag"].(string)
		if tag == "[sub1] 节点 A" || tag == "[sub2] 节点 B" {
			t.Errorf("subscription node %q should have been cleaned", tag)
		}
		// Check selector's outbounds list is also cleaned
		if tag == "my-selector" {
			if obList, ok := m["outbounds"].([]any); ok {
				for _, s := range obList {
					if str, ok := s.(string); ok {
						if str == "[sub1] 节点 A" || str == "[sub2] 节点 B" {
							t.Errorf("selector outbounds should not contain %q", str)
						}
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// InsertRealNodes
// ---------------------------------------------------------------------------

func TestUpdater_InsertRealNodes(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"type": "direct", "tag": "direct"},
		},
	}
	path := writeConfigFile(t, dir, "config.json", cfg)

	nodes := []map[string]any{
		{"type": "vmess", "tag": "[sub] 节点 A", "server": "1.2.3.4", "server_port": 443},
	}

	u := fileops.NewUpdater("")
	err := u.InsertRealNodes(path, nodes, []string{"sub"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := readConfigFile(t, path)
	outbounds := getOutbounds(t, result)

	found := false
	for _, ob := range outbounds {
		if m, ok := ob.(map[string]any); ok {
			if m["tag"] == "[sub] 节点 A" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("inserted node not found in outbounds")
	}
}

// ---------------------------------------------------------------------------
// CleanSubscriptionArtifacts (named subscriptions)
// ---------------------------------------------------------------------------

func TestUpdater_CleanSubscriptionArtifacts_ByName(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{
				"type":      "selector",
				"tag":       "my-selector",
				"outbounds": []any{"direct", "[sub1] 节点 A", "[sub2] 节点 B"},
			},
			map[string]any{"type": "vmess", "tag": "[sub1] 节点 A", "server": "1.2.3.4", "server_port": 443},
			map[string]any{"type": "vmess", "tag": "[sub2] 节点 B", "server": "5.6.7.8", "server_port": 443},
			map[string]any{"type": "direct", "tag": "direct"},
		},
	}
	path := writeConfigFile(t, dir, "config.json", cfg)

	u := fileops.NewUpdater("my-selector")
	// Only clean sub1, sub2 should remain
	err := u.CleanSubscriptionArtifacts(path, []string{"sub1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := readConfigFile(t, path)
	outbounds := getOutbounds(t, result)

	sub1Found := false
	sub2Found := false
	for _, ob := range outbounds {
		if m, ok := ob.(map[string]any); ok {
			switch m["tag"] {
			case "[sub1] 节点 A":
				sub1Found = true
			case "[sub2] 节点 B":
				sub2Found = true
			}
		}
	}
	if sub1Found {
		t.Error("[sub1] 节点 A should have been removed")
	}
	if !sub2Found {
		t.Error("[sub2] 节点 B should still be present")
	}
}

// ---------------------------------------------------------------------------
// ExpandRelayNodesByDetours
// ---------------------------------------------------------------------------

func TestUpdater_ExpandRelayNodesByDetours(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"type": "vmess", "tag": "[relay] 模板节点", "server": "relay.server", "server_port": 443},
			map[string]any{"type": "direct", "tag": "direct"},
		},
	}
	path := writeConfigFile(t, dir, "config.json", cfg)

	detourTags := []string{"[sub] 美国 01", "[sub] 日本 02"}

	u := fileops.NewUpdater("")
	generated, err := u.ExpandRelayNodesByDetours(path, []string{"relay"}, detourTags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should generate 2 nodes (1 template × 2 detours)
	if len(generated) != 2 {
		t.Errorf("expected 2 generated nodes, got %d", len(generated))
	}

	// Verify detour fields are set
	for _, n := range generated {
		if _, ok := n["detour"]; !ok {
			t.Error("generated node should have detour field")
		}
	}

	// Original template node should be gone from file
	result := readConfigFile(t, path)
	outbounds := getOutbounds(t, result)
	for _, ob := range outbounds {
		if m, ok := ob.(map[string]any); ok {
			if m["tag"] == "[relay] 模板节点" {
				t.Error("original relay template node should have been replaced")
			}
		}
	}
}

// ---------------------------------------------------------------------------
// AddDetourForSubscriptions
// ---------------------------------------------------------------------------

func TestUpdater_AddDetourForSubscriptions(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"type": "vmess", "tag": "[sub] 美国 01", "server": "1.2.3.4", "server_port": 443},
			map[string]any{"type": "vmess", "tag": "[other] 日本 02", "server": "5.6.7.8", "server_port": 443},
			map[string]any{"type": "direct", "tag": "direct"},
		},
	}
	path := writeConfigFile(t, dir, "config.json", cfg)

	u := fileops.NewUpdater("")
	err := u.AddDetourForSubscriptions(path, []string{"sub"}, "my-detour")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := readConfigFile(t, path)
	outbounds := getOutbounds(t, result)

	for _, ob := range outbounds {
		m, ok := ob.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := m["tag"].(string)
		if tag == "[sub] 美国 01" {
			if m["detour"] != "my-detour" {
				t.Errorf("expected detour=my-detour for [sub] node, got %v", m["detour"])
			}
		}
		if tag == "[other] 日本 02" {
			if _, ok := m["detour"]; ok {
				t.Error("[other] node should not have detour set")
			}
		}
	}
}
