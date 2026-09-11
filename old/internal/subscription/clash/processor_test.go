package clash_test

import (
	"fmt"
	"strings"
	"testing"

	"node-box/internal/subscription/clash"
)

// buildClashYAML is a helper to construct minimal Clash YAML for testing.
func buildClashYAML(proxies string) []byte {
	return []byte("proxies:\n" + proxies)
}

// ---------------------------------------------------------------------------
// Basic processing
// ---------------------------------------------------------------------------

func TestClashProcessor_Process_VMess(t *testing.T) {
	data := buildClashYAML(`  - name: vmess-node
    type: vmess
    server: 1.2.3.4
    port: "443"
    uuid: test-uuid
    alterId: 0
    cipher: auto
    tls: true
    servername: example.com
    network: ws
    ws-opts:
      path: /ws
      headers:
        Host: example.com
`)
	p := clash.NewClashProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 node")
	}
	n := nodes[0]
	if n["type"] != "vmess" {
		t.Errorf("expected type vmess, got %v", n["type"])
	}
	if n["tag"] != "vmess-node" {
		t.Errorf("expected tag vmess-node, got %v", n["tag"])
	}
}

func TestClashProcessor_Process_Shadowsocks(t *testing.T) {
	data := buildClashYAML(`  - name: ss-node
    type: ss
    server: 1.2.3.4
    port: "8388"
    cipher: aes-256-gcm
    password: mypassword
`)
	p := clash.NewClashProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 node")
	}
	n := nodes[0]
	if n["type"] != "shadowsocks" {
		t.Errorf("expected type shadowsocks, got %v", n["type"])
	}
	if n["tag"] != "ss-node" {
		t.Errorf("expected tag ss-node, got %v", n["tag"])
	}
}

func TestClashProcessor_Process_Trojan(t *testing.T) {
	data := buildClashYAML(`  - name: trojan-node
    type: trojan
    server: example.com
    port: "443"
    password: trojanpass
    sni: example.com
`)
	p := clash.NewClashProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 node")
	}
	n := nodes[0]
	if n["type"] != "trojan" {
		t.Errorf("expected type trojan, got %v", n["type"])
	}
}

func TestClashProcessor_Process_MultipleNodes(t *testing.T) {
	data := buildClashYAML(`  - name: node-1
    type: ss
    server: 1.1.1.1
    port: "8388"
    cipher: aes-256-gcm
    password: pass1
  - name: node-2
    type: ss
    server: 2.2.2.2
    port: "8388"
    cipher: chacha20-ietf-poly1305
    password: pass2
  - name: node-3
    type: trojan
    server: 3.3.3.3
    port: "443"
    password: pass3
`)
	p := clash.NewClashProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(nodes))
	}
}

// ---------------------------------------------------------------------------
// Error cases
// ---------------------------------------------------------------------------

func TestClashProcessor_Process_EmptyProxies(t *testing.T) {
	data := []byte("proxies: []\n")
	p := clash.NewClashProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestClashProcessor_Process_UnsupportedType_Skipped(t *testing.T) {
	// "ssr" is not supported; should be skipped without returning error
	data := buildClashYAML(`  - name: ssr-node
    type: ssr
    server: 1.2.3.4
    port: "1080"
    cipher: aes-256-cfb
    password: pass
    obfs: plain
    protocol: origin
  - name: ss-node
    type: ss
    server: 1.2.3.4
    port: "8388"
    cipher: aes-256-gcm
    password: pass
`)
	p := clash.NewClashProcessor()
	nodes, err := p.Process(data)
	// Error may be returned for unsupported type, but ss-node should still be processed
	_ = err
	found := false
	for _, n := range nodes {
		if n["tag"] == "ss-node" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ss-node should be present even when ssr-node is skipped")
	}
}

// ---------------------------------------------------------------------------
// VLESS with Reality
// ---------------------------------------------------------------------------

func TestClashProcessor_Process_VLESS_Reality(t *testing.T) {
	data := buildClashYAML(`  - name: vless-reality
    type: vless
    server: 1.2.3.4
    port: "443"
    uuid: test-uuid
    tls: true
    servername: example.com
    reality-opts:
      public-key: pubkey123
      short-id: sid123
    client-fingerprint: chrome
`)
	p := clash.NewClashProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 node")
	}
	n := nodes[0]
	if n["type"] != "vless" {
		t.Errorf("expected type vless, got %v", n["type"])
	}
	tls, ok := n["tls"].(map[string]any)
	if !ok {
		t.Fatal("expected tls field")
	}
	reality, ok := tls["reality"].(map[string]any)
	if !ok {
		t.Fatal("expected reality field in tls")
	}
	if reality["public_key"] != "pubkey123" {
		t.Errorf("expected public_key=pubkey123, got %v", reality["public_key"])
	}
}

// ---------------------------------------------------------------------------
// Hysteria2
// ---------------------------------------------------------------------------

func TestClashProcessor_Process_Hysteria2(t *testing.T) {
	data := buildClashYAML(`  - name: hy2-node
    type: hysteria2
    server: 1.2.3.4
    port: "443"
    password: hy2pass
    sni: example.com
`)
	p := clash.NewClashProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 node")
	}
	n := nodes[0]
	if n["type"] != "hysteria2" {
		t.Errorf("expected type hysteria2, got %v", n["type"])
	}
}

// ---------------------------------------------------------------------------
// Node field preservation
// ---------------------------------------------------------------------------

func TestClashProcessor_Process_PreservesServerAndPort(t *testing.T) {
	data := buildClashYAML(`  - name: check-fields
    type: ss
    server: 192.168.1.1
    port: "9999"
    cipher: chacha20-ietf-poly1305
    password: testpass
`)
	p := clash.NewClashProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected 1 node")
	}
	n := nodes[0]
	if n["server"] != "192.168.1.1" {
		t.Errorf("server not preserved: %v", n["server"])
	}
	portStr := strings.TrimSpace(fmt.Sprintf("%v", n["server_port"]))
	if portStr != "9999" {
		t.Errorf("server_port not preserved: %v", n["server_port"])
	}
}

// ---------------------------------------------------------------------------
// Relay proxy groups
// ---------------------------------------------------------------------------

func TestClashProcessor_Process_RelayGroup(t *testing.T) {
	data := []byte(`proxies:
  - name: node-a
    type: ss
    server: 1.1.1.1
    port: "8388"
    cipher: aes-256-gcm
    password: pass
  - name: node-b
    type: ss
    server: 2.2.2.2
    port: "8388"
    cipher: aes-256-gcm
    password: pass
proxy-groups:
  - name: relay-chain
    type: relay
    proxies:
      - node-a
      - node-b
`)
	p := clash.NewClashProcessor()
	nodes, err := p.Process(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have at least the 2 base nodes
	if len(nodes) < 2 {
		t.Errorf("expected at least 2 nodes, got %d", len(nodes))
	}
}
