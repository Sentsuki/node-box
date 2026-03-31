package xray

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)


// parseLink parses a single sharing link and returns a SingBox outbound node.
func parseLink(link string) (map[string]any, error) {
	link = strings.TrimSpace(link)
	if link == "" {
		return nil, fmt.Errorf("empty link")
	}

	// VMess uses a special base64-json format, handle before url.Parse
	if strings.HasPrefix(link, "vmess://") {
		return parseVMess(link)
	}

	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	switch u.Scheme {
	case "vless":
		return parseVLESS(u)
	case "ss":
		return parseSS(u)
	case "trojan":
		return parseTrojan(u)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", u.Scheme)
	}
}

// --- VMess ---

func parseVMess(link string) (map[string]any, error) {
	raw := strings.TrimPrefix(link, "vmess://")
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		// Try without padding
		b, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("vmess base64 decode: %w", err)
		}
	}

	var v struct {
		Ps   string `json:"ps"`
		Add  string `json:"add"`
		Port any    `json:"port"`
		Id   string `json:"id"`
		Aid  any    `json:"aid"`
		Scy  string `json:"scy"`
		Net  string `json:"net"`
		Type string `json:"type"`
		Host string `json:"host"`
		Path string `json:"path"`
		Tls  string `json:"tls"`
		Sni  string `json:"sni"`
		Alpn string `json:"alpn"`
		Fp   string `json:"fp"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("vmess json decode: %w", err)
	}

	port := parseAnyPort(v.Port)
	if port == 0 {
		return nil, fmt.Errorf("vmess: invalid port")
	}

	alterId := 0
	switch aid := v.Aid.(type) {
	case string:
		if i, err := strconv.Atoi(aid); err == nil {
			alterId = i
		}
	case float64:
		alterId = int(aid)
	}

	security := v.Scy
	if security == "" {
		security = "auto"
	}

	node := map[string]any{
		"type":        "vmess",
		"tag":         v.Ps,
		"server":      v.Add,
		"server_port": port,
		"uuid":        v.Id,
		"alter_id":    alterId,
		"security":    security,
	}

	// TLS
	if v.Tls == "tls" {
		tls := map[string]any{
			"enabled": true,
		}
		if v.Sni != "" {
			tls["server_name"] = v.Sni
		}
		if v.Fp != "" {
			tls["utls"] = map[string]any{
				"enabled":     true,
				"fingerprint": v.Fp,
			}
		}
		if v.Alpn != "" {
			tls["alpn"] = strings.Split(v.Alpn, ",")
		}
		node["tls"] = tls
	}

	// Transport
	setTransport(node, v.Net, v.Host, v.Path, "")

	return node, nil
}

// --- VLESS ---

func parseVLESS(u *url.URL) (map[string]any, error) {
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, fmt.Errorf("vless: invalid port: %w", err)
	}

	uuid := ""
	if u.User != nil {
		uuid = u.User.String()
	}

	node := map[string]any{
		"type":        "vless",
		"tag":         decodeFragment(u.Fragment),
		"server":      u.Hostname(),
		"server_port": port,
		"uuid":        uuid,
	}

	q := u.Query()

	// TLS
	security := q.Get("security")
	if security != "" && security != "none" {
		tls := map[string]any{
			"enabled": true,
		}

		sni := q.Get("sni")
		if sni == "" {
			sni = q.Get("peer")
		}
		if sni != "" {
			tls["server_name"] = sni
		}

		if q.Get("fp") != "" {
			tls["utls"] = map[string]any{
				"enabled":     true,
				"fingerprint": q.Get("fp"),
			}
		}

		if alpn := q.Get("alpn"); alpn != "" {
			tls["alpn"] = strings.Split(alpn, ",")
		}

		if queryBool(q.Get("allowInsecure")) || queryBool(q.Get("allowinsecure")) {
			tls["insecure"] = true
		}

		// Reality
		if security == "reality" {
			reality := map[string]any{
				"enabled": true,
			}
			if pbk := q.Get("pbk"); pbk != "" {
				reality["public_key"] = pbk
			}
			if sid := q.Get("sid"); sid != "" {
				reality["short_id"] = sid
			}
			tls["reality"] = reality
		}

		node["tls"] = tls
	}

	// Flow (only for non-ws transport)
	flow := q.Get("flow")
	network := q.Get("type")
	if flow != "" && network != "ws" {
		node["flow"] = flow
	}

	// Packet encoding
	if pe := q.Get("packetEncoding"); pe != "" {
		node["packet_encoding"] = pe
	}

	// Transport
	host := q.Get("host")
	path := q.Get("path")
	serviceName := q.Get("serviceName")
	setTransport(node, network, host, path, serviceName)

	return node, nil
}

// --- Shadowsocks ---

func parseSS(u *url.URL) (map[string]any, error) {
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, fmt.Errorf("ss: invalid port: %w", err)
	}

	var method, password string

	// Try SIP002 format: method:password@host:port
	if u.User != nil {
		pwd, hasPwd := u.User.Password()
		if hasPwd {
			method = u.User.Username()
			password = pwd
		} else {
			// Legacy format: base64(method:password)@host:port
			decoded, decErr := base64.RawURLEncoding.DecodeString(u.User.Username())
			if decErr == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 {
					method = parts[0]
					password = parts[1]
				}
			}
		}
	}

	if method == "" || password == "" {
		return nil, fmt.Errorf("ss: cannot parse method/password")
	}

	node := map[string]any{
		"type":        "shadowsocks",
		"tag":         decodeFragment(u.Fragment),
		"server":      u.Hostname(),
		"server_port": port,
		"method":      method,
		"password":    password,
	}

	return node, nil
}

// --- Trojan ---

func parseTrojan(u *url.URL) (map[string]any, error) {
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, fmt.Errorf("trojan: invalid port: %w", err)
	}

	password := ""
	if u.User != nil {
		password = u.User.Username()
	}

	node := map[string]any{
		"type":        "trojan",
		"tag":         decodeFragment(u.Fragment),
		"server":      u.Hostname(),
		"server_port": port,
		"password":    password,
	}

	q := u.Query()

	// Trojan is always TLS
	tls := map[string]any{
		"enabled": true,
	}

	sni := q.Get("sni")
	if sni != "" {
		tls["server_name"] = sni
	}

	if fp := q.Get("fp"); fp != "" {
		tls["utls"] = map[string]any{
			"enabled":     true,
			"fingerprint": fp,
		}
	}

	if alpn := q.Get("alpn"); alpn != "" {
		tls["alpn"] = strings.Split(alpn, ",")
	}

	if queryBool(q.Get("allowInsecure")) || queryBool(q.Get("allowinsecure")) ||
		queryBool(q.Get("skip-cert-verify")) {
		tls["insecure"] = true
	}

	node["tls"] = tls

	// Transport
	network := q.Get("type")
	host := q.Get("host")
	path := q.Get("path")
	serviceName := q.Get("serviceName")
	setTransport(node, network, host, path, serviceName)

	return node, nil
}

// --- Helpers ---

// setTransport sets the transport field on the node based on network type.
func setTransport(node map[string]any, network, host, path, serviceName string) {
	switch network {
	case "ws":
		transport := map[string]any{
			"type": "ws",
		}
		if path != "" {
			transport["path"] = path
		}
		if host != "" {
			transport["headers"] = map[string]any{
				"Host": []string{host},
			}
		}
		node["transport"] = transport
	case "grpc":
		transport := map[string]any{
			"type": "grpc",
		}
		sn := serviceName
		if sn == "" {
			sn = path
		}
		if sn != "" {
			transport["service_name"] = sn
		}
		node["transport"] = transport
	case "h2", "http":
		transport := map[string]any{
			"type": "http",
		}
		if path != "" {
			transport["path"] = path
		}
		if host != "" {
			transport["host"] = []string{host}
		}
		node["transport"] = transport
	case "httpupgrade":
		transport := map[string]any{
			"type": "httpupgrade",
		}
		if path != "" {
			transport["path"] = path
		}
		if host != "" {
			transport["host"] = host
		}
		node["transport"] = transport
	}
}

// parseAnyPort converts a JSON port value (string or float64) to int.
func parseAnyPort(v any) int {
	switch p := v.(type) {
	case string:
		i, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		return i
	case float64:
		return int(p)
	default:
		return 0
	}
}

// decodeFragment URL-decodes a fragment string for the node name.
func decodeFragment(fragment string) string {
	decoded, err := url.QueryUnescape(fragment)
	if err != nil {
		return fragment
	}
	return decoded
}
