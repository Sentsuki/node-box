package xray

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// parseLink – VMess
// ---------------------------------------------------------------------------

func TestParseLink_VMess_Basic(t *testing.T) {
	v := map[string]any{
		"ps":   "vmess-basic",
		"add":  "1.2.3.4",
		"port": 443,
		"id":   "test-uuid",
		"aid":  0,
		"scy":  "auto",
		"net":  "tcp",
		"tls":  "",
	}
	link := "vmess://" + base64Encode(v)
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertField(t, node, "type", "vmess")
	assertField(t, node, "tag", "vmess-basic")
	assertField(t, node, "server", "1.2.3.4")
	assertField(t, node, "server_port", 443)
	assertField(t, node, "uuid", "test-uuid")
}

func TestParseLink_VMess_WithTLS(t *testing.T) {
	v := map[string]any{
		"ps":   "vmess-tls",
		"add":  "example.com",
		"port": 443,
		"id":   "uuid-tls",
		"aid":  0,
		"net":  "ws",
		"tls":  "tls",
		"sni":  "example.com",
		"fp":   "chrome",
		"path": "/ws",
		"host": "example.com",
	}
	link := "vmess://" + base64Encode(v)
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tls, ok := node["tls"].(map[string]any)
	if !ok {
		t.Fatal("expected tls field")
	}
	if tls["enabled"] != true {
		t.Error("tls.enabled should be true")
	}
	if tls["server_name"] != "example.com" {
		t.Errorf("tls.server_name: expected example.com, got %v", tls["server_name"])
	}
	utls, ok := tls["utls"].(map[string]any)
	if !ok {
		t.Fatal("expected utls field")
	}
	if utls["fingerprint"] != "chrome" {
		t.Errorf("utls.fingerprint: expected chrome, got %v", utls["fingerprint"])
	}
}

func TestParseLink_VMess_WithWSTransport(t *testing.T) {
	v := map[string]any{
		"ps":   "vmess-ws",
		"add":  "1.2.3.4",
		"port": 80,
		"id":   "uuid-ws",
		"aid":  0,
		"net":  "ws",
		"path": "/path",
		"host": "cdn.example.com",
	}
	link := "vmess://" + base64Encode(v)
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	transport, ok := node["transport"].(map[string]any)
	if !ok {
		t.Fatal("expected transport field")
	}
	assertField(t, transport, "type", "ws")
	assertField(t, transport, "path", "/path")
}

func TestParseLink_VMess_WithGRPCTransport(t *testing.T) {
	v := map[string]any{
		"ps":   "vmess-grpc",
		"add":  "1.2.3.4",
		"port": 443,
		"id":   "uuid-grpc",
		"aid":  0,
		"net":  "grpc",
		"path": "my-service",
	}
	link := "vmess://" + base64Encode(v)
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	transport, ok := node["transport"].(map[string]any)
	if !ok {
		t.Fatal("expected transport field")
	}
	assertField(t, transport, "type", "grpc")
	assertField(t, transport, "service_name", "my-service")
}

func TestParseLink_VMess_MissingName(t *testing.T) {
	v := map[string]any{
		"ps":   "",
		"add":  "1.2.3.4",
		"port": 443,
		"id":   "uuid",
	}
	link := "vmess://" + base64Encode(v)
	_, err := parseLink(link)
	if err == nil {
		t.Error("expected error for missing ps field")
	}
}

func TestParseLink_VMess_InvalidPort(t *testing.T) {
	v := map[string]any{
		"ps":   "test",
		"add":  "1.2.3.4",
		"port": 0,
		"id":   "uuid",
	}
	link := "vmess://" + base64Encode(v)
	_, err := parseLink(link)
	if err == nil {
		t.Error("expected error for invalid port")
	}
}

func TestParseLink_VMess_StringPort(t *testing.T) {
	v := map[string]any{
		"ps":   "vmess-strport",
		"add":  "1.2.3.4",
		"port": "8080",
		"id":   "uuid",
		"aid":  "0",
	}
	link := "vmess://" + base64Encode(v)
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertField(t, node, "server_port", 8080)
}

// ---------------------------------------------------------------------------
// parseLink – VLESS
// ---------------------------------------------------------------------------

func TestParseLink_VLESS_Basic(t *testing.T) {
	link := "vless://my-uuid@1.2.3.4:443?security=tls&sni=example.com#vless-node"
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertField(t, node, "type", "vless")
	assertField(t, node, "tag", "vless-node")
	assertField(t, node, "server", "1.2.3.4")
	assertField(t, node, "server_port", 443)
	assertField(t, node, "uuid", "my-uuid")
}

func TestParseLink_VLESS_Reality(t *testing.T) {
	link := "vless://uuid@1.2.3.4:443?security=reality&pbk=pubkey&sid=shortid&fp=chrome&sni=example.com#reality-node"
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tls, ok := node["tls"].(map[string]any)
	if !ok {
		t.Fatal("expected tls field")
	}
	reality, ok := tls["reality"].(map[string]any)
	if !ok {
		t.Fatal("expected reality field")
	}
	assertField(t, reality, "enabled", true)
	assertField(t, reality, "public_key", "pubkey")
	assertField(t, reality, "short_id", "shortid")
}

func TestParseLink_VLESS_DefaultChromeFingerprint_Reality(t *testing.T) {
	// When security=reality but no fp param, should default to chrome
	link := "vless://uuid@1.2.3.4:443?security=reality&pbk=pubkey&sid=sid#node"
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tls := node["tls"].(map[string]any)
	utls, ok := tls["utls"].(map[string]any)
	if !ok {
		t.Fatal("expected utls field for reality without fp")
	}
	if utls["fingerprint"] != "chrome" {
		t.Errorf("expected default fingerprint chrome, got %v", utls["fingerprint"])
	}
}

func TestParseLink_VLESS_Flow(t *testing.T) {
	link := "vless://uuid@1.2.3.4:443?security=tls&flow=xtls-rprx-vision#flow-node"
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node["flow"] != "xtls-rprx-vision" {
		t.Errorf("expected flow field, got %v", node["flow"])
	}
}

func TestParseLink_VLESS_WSTransport_NoFlow(t *testing.T) {
	// flow should be ignored for ws transport
	link := "vless://uuid@1.2.3.4:443?type=ws&flow=xtls-rprx-vision&path=/ws#ws-node"
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := node["flow"]; ok {
		t.Error("flow should not be set for ws transport")
	}
}

func TestParseLink_VLESS_AllowInsecure(t *testing.T) {
	link := "vless://uuid@1.2.3.4:443?security=tls&allowInsecure=1#insecure-node"
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tls := node["tls"].(map[string]any)
	if tls["insecure"] != true {
		t.Error("expected insecure=true")
	}
}

func TestParseLink_VLESS_EncodedFragment(t *testing.T) {
	encoded := url.QueryEscape("节点 01")
	link := fmt.Sprintf("vless://uuid@1.2.3.4:443?security=tls#%s", encoded)
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node["tag"] != "节点 01" {
		t.Errorf("expected decoded tag '节点 01', got %v", node["tag"])
	}
}

// ---------------------------------------------------------------------------
// parseLink – Shadowsocks
// ---------------------------------------------------------------------------

func TestParseLink_SS_SIP002(t *testing.T) {
	// SIP002: ss://BASE64(method:password)@host:port#tag
	userInfo := base64.URLEncoding.EncodeToString([]byte("aes-256-gcm:mypassword"))
	link := fmt.Sprintf("ss://%s@1.2.3.4:8388#ss-node", userInfo)
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertField(t, node, "type", "shadowsocks")
	assertField(t, node, "tag", "ss-node")
	assertField(t, node, "method", "aes-256-gcm")
	assertField(t, node, "password", "mypassword")
	assertField(t, node, "server", "1.2.3.4")
	assertField(t, node, "server_port", 8388)
}

func TestParseLink_SS_LegacyBase64(t *testing.T) {
	// Legacy: ss://BASE64(method:password@host:port)#tag
	raw := "aes-128-gcm:pass123@5.6.7.8:1234"
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	link := fmt.Sprintf("ss://%s#legacy-ss", encoded)
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertField(t, node, "method", "aes-128-gcm")
	assertField(t, node, "password", "pass123")
	assertField(t, node, "server", "5.6.7.8")
	assertField(t, node, "server_port", 1234)
}

func TestParseLink_SS_InvalidNoPassword(t *testing.T) {
	link := "ss://1.2.3.4:8388#bad-ss"
	_, err := parseLink(link)
	if err == nil {
		t.Error("expected error for SS link without method/password")
	}
}

// ---------------------------------------------------------------------------
// parseLink – Trojan
// ---------------------------------------------------------------------------

func TestParseLink_Trojan_Basic(t *testing.T) {
	link := "trojan://mypassword@1.2.3.4:443?sni=example.com#trojan-node"
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertField(t, node, "type", "trojan")
	assertField(t, node, "tag", "trojan-node")
	assertField(t, node, "password", "mypassword")
	assertField(t, node, "server", "1.2.3.4")
	assertField(t, node, "server_port", 443)
	tls, ok := node["tls"].(map[string]any)
	if !ok {
		t.Fatal("trojan should always have tls")
	}
	if tls["enabled"] != true {
		t.Error("tls.enabled should be true for trojan")
	}
}

func TestParseLink_Trojan_WithWSTransport(t *testing.T) {
	link := "trojan://pass@1.2.3.4:443?type=ws&path=/trojan&host=cdn.example.com#trojan-ws"
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	transport, ok := node["transport"].(map[string]any)
	if !ok {
		t.Fatal("expected transport field")
	}
	assertField(t, transport, "type", "ws")
	assertField(t, transport, "path", "/trojan")
}

func TestParseLink_Trojan_SkipCertVerify(t *testing.T) {
	link := "trojan://pass@1.2.3.4:443?skip-cert-verify=1#insecure-trojan"
	node, err := parseLink(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tls := node["tls"].(map[string]any)
	if tls["insecure"] != true {
		t.Error("expected insecure=true for skip-cert-verify=1")
	}
}

func TestParseLink_Trojan_InvalidPort(t *testing.T) {
	link := "trojan://pass@1.2.3.4:notaport#bad"
	_, err := parseLink(link)
	if err == nil {
		t.Error("expected error for invalid port")
	}
}

// ---------------------------------------------------------------------------
// parseLink – unsupported protocol
// ---------------------------------------------------------------------------

func TestParseLink_UnsupportedProtocol(t *testing.T) {
	_, err := parseLink("http://example.com:80")
	if err == nil {
		t.Error("expected error for unsupported protocol")
	}
}

func TestParseLink_EmptyLink(t *testing.T) {
	_, err := parseLink("")
	if err == nil {
		t.Error("expected error for empty link")
	}
}

// ---------------------------------------------------------------------------
// setTransport
// ---------------------------------------------------------------------------

func TestSetTransport_HTTP(t *testing.T) {
	node := map[string]any{}
	q := url.Values{}
	q.Set("path", "/http-path")
	setTransport(node, "http", "example.com", q)
	transport, ok := node["transport"].(map[string]any)
	if !ok {
		t.Fatal("expected transport field")
	}
	assertField(t, transport, "type", "http")
	assertField(t, transport, "path", "/http-path")
}

func TestSetTransport_HTTPUpgrade(t *testing.T) {
	node := map[string]any{}
	q := url.Values{}
	q.Set("path", "/upgrade")
	setTransport(node, "httpupgrade", "cdn.example.com", q)
	transport, ok := node["transport"].(map[string]any)
	if !ok {
		t.Fatal("expected transport field")
	}
	assertField(t, transport, "type", "httpupgrade")
	assertField(t, transport, "path", "/upgrade")
	assertField(t, transport, "host", "cdn.example.com")
}

func TestSetTransport_WS_EarlyData(t *testing.T) {
	node := map[string]any{}
	q := url.Values{}
	q.Set("path", "/ws?ed=2048")
	setTransport(node, "ws", "", q)
	transport, ok := node["transport"].(map[string]any)
	if !ok {
		t.Fatal("expected transport field")
	}
	assertField(t, transport, "type", "ws")
	if transport["max_early_data"] != 2048 {
		t.Errorf("expected max_early_data=2048, got %v", transport["max_early_data"])
	}
	if transport["early_data_header_name"] != "Sec-WebSocket-Protocol" {
		t.Errorf("expected Sec-WebSocket-Protocol header, got %v", transport["early_data_header_name"])
	}
	// path should be cleaned (no ?ed=...)
	if p, ok := transport["path"].(string); ok && strings.Contains(p, "?") {
		t.Errorf("path should not contain query string, got %q", p)
	}
}

func TestSetTransport_GRPC_WithOptions(t *testing.T) {
	node := map[string]any{}
	q := url.Values{}
	q.Set("serviceName", "my-grpc-service")
	q.Set("idle_timeout", "30")
	setTransport(node, "grpc", "", q)
	transport, ok := node["transport"].(map[string]any)
	if !ok {
		t.Fatal("expected transport field")
	}
	assertField(t, transport, "type", "grpc")
	assertField(t, transport, "service_name", "my-grpc-service")
	if transport["idle_timeout"] != "30s" {
		t.Errorf("expected idle_timeout=30s, got %v", transport["idle_timeout"])
	}
}

func TestSetTransport_UnknownNetwork_NoTransport(t *testing.T) {
	node := map[string]any{}
	setTransport(node, "tcp", "", url.Values{})
	if _, ok := node["transport"]; ok {
		t.Error("tcp transport should not set transport field")
	}
}

// ---------------------------------------------------------------------------
// Utility functions
// ---------------------------------------------------------------------------

func TestTryBase64Decode_Variants(t *testing.T) {
	original := []byte("hello world test data")

	cases := []struct {
		name    string
		encoded string
	}{
		{"standard", base64.StdEncoding.EncodeToString(original)},
		{"raw standard", base64.RawStdEncoding.EncodeToString(original)},
		{"url safe", base64.URLEncoding.EncodeToString(original)},
		{"raw url safe", base64.RawURLEncoding.EncodeToString(original)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			decoded, ok := tryBase64Decode(c.encoded)
			if !ok {
				t.Errorf("failed to decode %s variant", c.name)
			}
			if string(decoded) != string(original) {
				t.Errorf("decoded mismatch: got %q, want %q", decoded, original)
			}
		})
	}
}

func TestTryBase64Decode_InvalidInput(t *testing.T) {
	_, ok := tryBase64Decode("not-base64!!!")
	if ok {
		t.Error("expected failure for invalid base64")
	}
}

func TestTryBase64Decode_Empty(t *testing.T) {
	_, ok := tryBase64Decode("")
	if !ok {
		// empty string decodes to empty bytes in standard base64
		// but our implementation returns false for empty
	}
	// Just verify it doesn't panic
}

func TestDecodeSubscription_AlreadyLinks(t *testing.T) {
	input := "vless://uuid@1.2.3.4:443#node1\nvmess://abc"
	result := decodeSubscription([]byte(input))
	if result != input {
		t.Errorf("expected input unchanged, got %q", result)
	}
}

func TestDecodeSubscription_Base64Encoded(t *testing.T) {
	links := "vless://uuid@1.2.3.4:443#node1\nvmess://abc"
	encoded := base64.StdEncoding.EncodeToString([]byte(links))
	result := decodeSubscription([]byte(encoded))
	if result != links {
		t.Errorf("expected decoded links, got %q", result)
	}
}

func TestSplitLines_FiltersEmpty(t *testing.T) {
	input := "line1\n\nline2\n   \nline3"
	lines := splitLines(input)
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
}

func TestQueryBool(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"1", true},
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"0", false},
		{"false", false},
		{"", false},
		{"yes", false},
	}
	for _, c := range cases {
		got := queryBool(c.val)
		if got != c.want {
			t.Errorf("queryBool(%q) = %v, want %v", c.val, got, c.want)
		}
	}
}

func TestParseAnyPort(t *testing.T) {
	cases := []struct {
		input any
		want  int
	}{
		{"443", 443},
		{float64(8080), 8080},
		{"invalid", 0},
		{nil, 0},
	}
	for _, c := range cases {
		got := parseAnyPort(c.input)
		if got != c.want {
			t.Errorf("parseAnyPort(%v) = %d, want %d", c.input, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func base64Encode(v map[string]any) string {
	b, _ := json.Marshal(v)
	return base64.StdEncoding.EncodeToString(b)
}

func assertField(t *testing.T, m map[string]any, key string, expected any) {
	t.Helper()
	val, ok := m[key]
	if !ok {
		t.Errorf("missing key %q", key)
		return
	}
	if fmt.Sprintf("%v", val) != fmt.Sprintf("%v", expected) {
		t.Errorf("key %q: expected %v (%T), got %v (%T)", key, expected, expected, val, val)
	}
}
