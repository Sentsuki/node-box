package singbox_test

import (
	"encoding/json"
	"testing"

	"node-box/internal/subscription/singbox"
)

func buildSingBoxJSON(outbounds []map[string]any) []byte {
	cfg := map[string]any{
		"outbounds": outbounds,
	}
	b, _ := json.Marshal(cfg)
	return b
}

// ---------------------------------------------------------------------------
// Basic processing
// ---------------------------------------------------------------------------

func TestSingBoxProcessor_Process_ExtractsProxyNodes(t *testing.T) {
	data := buildSingBoxJSON([]map[string]any{
		{"type": "vmess", "tag": "vmess-node", "server": "1.2.3.4", "server_port": 443},
		{"type": "vless", "tag": "vless-node", "server": "5.6.7.8", "server_port": 443},
		{"type": "shadowsocks", "tag": "ss-node", "server": "9.10.11.12", "server_port": 8388},
	})

	p := singbox.NewSingBoxProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(nodes))
	}
}

func TestSingBoxProcessor_Process_FiltersSystemOutbounds(t *testing.T) {
	data := buildSingBoxJSON([]map[string]any{
		{"type": "direct", "tag": "direct"},
		{"type": "block", "tag": "block"},
		{"type": "selector", "tag": "select", "outbounds": []string{"vmess-node"}},
		{"type": "urltest", "tag": "auto", "outbounds": []string{"vmess-node"}},
		{"type": "vmess", "tag": "vmess-node", "server": "1.2.3.4", "server_port": 443},
	})

	p := singbox.NewSingBoxProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 proxy node, got %d", len(nodes))
	}
	if nodes[0]["tag"] != "vmess-node" {
		t.Errorf("expected vmess-node, got %v", nodes[0]["tag"])
	}
}

func TestSingBoxProcessor_Process_PreservesAllFields(t *testing.T) {
	data := buildSingBoxJSON([]map[string]any{
		{
			"type":        "trojan",
			"tag":         "trojan-node",
			"server":      "example.com",
			"server_port": 443,
			"password":    "secret",
			"tls": map[string]any{
				"enabled":     true,
				"server_name": "example.com",
			},
		},
	})

	p := singbox.NewSingBoxProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n["password"] != "secret" {
		t.Errorf("password field not preserved: %v", n["password"])
	}
	if n["server"] != "example.com" {
		t.Errorf("server field not preserved: %v", n["server"])
	}
	if _, ok := n["tls"]; !ok {
		t.Error("tls field not preserved")
	}
}

func TestSingBoxProcessor_Process_EmptyOutbounds(t *testing.T) {
	data := buildSingBoxJSON([]map[string]any{
		{"type": "direct", "tag": "direct"},
	})

	p := singbox.NewSingBoxProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 proxy nodes, got %d", len(nodes))
	}
}

// ---------------------------------------------------------------------------
// Error cases
// ---------------------------------------------------------------------------

func TestSingBoxProcessor_Process_InvalidJSON(t *testing.T) {
	p := singbox.NewSingBoxProcessor()
	_, err := p.Process([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSingBoxProcessor_Process_MissingOutbounds(t *testing.T) {
	data, _ := json.Marshal(map[string]any{"log": map[string]any{"level": "info"}})
	p := singbox.NewSingBoxProcessor()
	_, err := p.Process(data)
	if err == nil {
		t.Error("expected error when outbounds field is missing")
	}
}

func TestSingBoxProcessor_Process_InvalidOutboundsFormat(t *testing.T) {
	data, _ := json.Marshal(map[string]any{"outbounds": "not-an-array"})
	p := singbox.NewSingBoxProcessor()
	_, err := p.Process(data)
	if err == nil {
		t.Error("expected error when outbounds is not an array")
	}
}

func TestSingBoxProcessor_Process_EmptyData(t *testing.T) {
	p := singbox.NewSingBoxProcessor()
	_, err := p.Process([]byte("{}"))
	if err == nil {
		t.Error("expected error for empty config without outbounds")
	}
}

// ---------------------------------------------------------------------------
// Node type coverage
// ---------------------------------------------------------------------------

func TestSingBoxProcessor_Process_MultipleProxyTypes(t *testing.T) {
	types := []string{"vmess", "vless", "shadowsocks", "trojan", "hysteria2", "tuic"}
	var outbounds []map[string]any
	for _, typ := range types {
		outbounds = append(outbounds, map[string]any{
			"type":        typ,
			"tag":         typ + "-node",
			"server":      "1.2.3.4",
			"server_port": 443,
		})
	}
	// Add system outbounds that should be filtered
	outbounds = append(outbounds,
		map[string]any{"type": "direct", "tag": "direct"},
		map[string]any{"type": "block", "tag": "block"},
	)

	data := buildSingBoxJSON(outbounds)
	p := singbox.NewSingBoxProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != len(types) {
		t.Errorf("expected %d proxy nodes, got %d", len(types), len(nodes))
	}
}
