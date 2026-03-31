package xray

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessor_Process_VMess(t *testing.T) {
	xp := NewXrayProcessor()

	// Normal VMess JSON
	vmessData := map[string]any{
		"v":    "2",
		"ps":   "test-vmess",
		"add":  "1.2.3.4",
		"port": 443,
		"id":   "uuid-test",
		"aid":  0,
		"scy":  "auto",
		"net":  "ws",
		"type": "none",
		"host": "example.com",
		"path": "/ws",
		"tls":  "tls",
		"sni":  "example.com",
		"fp":   "chrome",
	}
	bytes, _ := json.Marshal(vmessData)
	link := "vmess://" + base64.StdEncoding.EncodeToString(bytes)

	nodes, err := xp.Process([]byte(link))
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Fatalf("Expected 1 node, got %d", len(nodes))
	}

	n := nodes[0]
	assertValue(t, n, "type", "vmess")
	assertValue(t, n, "tag", "test-vmess")
	assertValue(t, n, "server", "1.2.3.4")
	assertValue(t, n, "server_port", 443)
	assertValue(t, n, "uuid", "uuid-test")
}

func TestProcessor_Process_VLESS(t *testing.T) {
	xp := NewXrayProcessor()

	link := "vless://uuid-test@1.2.3.4:443?type=grpc&serviceName=test-grpc&security=reality&pbk=public-key&sid=short-id&sni=example.com#test-vless"

	nodes, err := xp.Process([]byte(link))
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Fatalf("Expected 1 node, got %d", len(nodes))
	}

	n := nodes[0]
	assertValue(t, n, "type", "vless")
	assertValue(t, n, "tag", "test-vless")
	assertValue(t, n, "server", "1.2.3.4")
	assertValue(t, n, "server_port", 443)

	// Check transport
	transport, ok := n["transport"].(map[string]any)
	if !ok {
		t.Errorf("Expected transport field")
	} else {
		assertValue(t, transport, "type", "grpc")
		assertValue(t, transport, "service_name", "test-grpc")
	}

	// Check reality
	tls, ok := n["tls"].(map[string]any)
	if !ok {
		t.Errorf("Expected tls field")
	} else {
		reality, ok := tls["reality"].(map[string]any)
		if !ok {
			t.Errorf("Expected reality field in tls")
		} else {
			assertValue(t, reality, "enabled", true)
			assertValue(t, reality, "public_key", "public-key")
		}
	}
}

func TestProcessor_Process_Shadowsocks(t *testing.T) {
	xp := NewXrayProcessor()

	// SIP002 format
	methodPass := base64.URLEncoding.EncodeToString([]byte("aes-256-gcm:password-test"))
	link := fmt.Sprintf("ss://%s@1.2.3.4:8388#test-ss", methodPass)

	nodes, err := xp.Process([]byte(link))
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Fatalf("Expected 1 node, got %d", len(nodes))
	}

	n := nodes[0]
	assertValue(t, n, "type", "shadowsocks")
	assertValue(t, n, "tag", "test-ss")
	assertValue(t, n, "method", "aes-256-gcm")
	assertValue(t, n, "password", "password-test")
}

func TestProcessor_Process_Trojan(t *testing.T) {
	xp := NewXrayProcessor()

	link := "trojan://password-test@1.2.3.4:443?security=tls&sni=example.com&type=ws&path=/trojan#test-trojan"

	nodes, err := xp.Process([]byte(link))
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Fatalf("Expected 1 node, got %d", len(nodes))
	}

	n := nodes[0]
	assertValue(t, n, "type", "trojan")
	assertValue(t, n, "tag", "test-trojan")
	assertValue(t, n, "password", "password-test")

	// Check transport
	transport, ok := n["transport"].(map[string]any)
	if !ok {
		t.Errorf("Expected transport field")
	} else {
		assertValue(t, transport, "type", "ws")
		assertValue(t, transport, "path", "/trojan")
	}
}

func TestProcessor_Process_Base64Subscription(t *testing.T) {
	xp := NewXrayProcessor()

	links := []string{
		"vless://uuid1@1.1.1.1:443?security=tls#node1",
		"ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ=@2.2.2.2:8388#node2",
	}
	subscriptionData := base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))

	nodes, err := xp.Process([]byte(subscriptionData))
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if len(nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(nodes))
	}

	assertValue(t, nodes[0], "tag", "node1")
	assertValue(t, nodes[1], "tag", "node2")
}

// TestProcessor_Manual is for manually testing real subscription data.
// It reads from a "test_data.txt" file in the same directory.
func TestProcessor_Manual(t *testing.T) {
	path := "test_data.txt"
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("Skipping manual test: test_data.txt not found. Create it to test real data.")
		}
		t.Fatalf("Failed to read test_data.txt: %v", err)
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		t.Skip("Skipping manual test: test_data.txt is empty.")
	}

	xp := NewXrayProcessor()
	nodes, err := xp.Process([]byte(content))
	if err != nil {
		t.Fatalf("Manual process failed: %v", err)
	}

	t.Logf("Successfully parsed %d nodes from test_data.txt", len(nodes))
	
	// Print the first few nodes tags for verification
	maxPrint := 5
	if len(nodes) < maxPrint {
		maxPrint = len(nodes)
	}
	for i := 0; i < maxPrint; i++ {
		t.Logf("Node [%d]: %v (Type: %v)", i, nodes[i]["tag"], nodes[i]["type"])
	}

	// Print to a JSON file for deep inspection
	resultsPath := "test_results.json"
	out, _ := json.MarshalIndent(nodes, "", "  ")
	if err := os.WriteFile(resultsPath, out, 0644); err == nil {
		absPath, _ := filepath.Abs(resultsPath)
		t.Logf("Full results written to: %s", absPath)
	}
}

// Helper
func assertValue(t *testing.T, m map[string]any, key string, expected any) {
	val, ok := m[key]
	if !ok {
		t.Errorf("Missing key: %s", key)
		return
	}
	if fmt.Sprintf("%v", val) != fmt.Sprintf("%v", expected) {
		t.Errorf("Key %s: expected %v, got %v", key, expected, val)
	}
}
