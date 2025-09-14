package subscription

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ClashProcessor handles Clash subscription data processing.
// It converts Clash format proxy configurations to SingBox format nodes.
type ClashProcessor struct{}

// NewClashProcessor creates a new Clash processor instance.
// Returns a processor that can handle Clash format subscription data.
func NewClashProcessor() *ClashProcessor {
	return &ClashProcessor{}
}

// Process handles Clash subscription data and converts it to SingBox format nodes.
// It parses the YAML data, extracts proxy configurations, and converts each
// proxy to the unified Node format compatible with SingBox.
func (cp *ClashProcessor) Process(data []byte) ([]Node, error) {
	var clashConfig ClashConfig
	if err := yaml.Unmarshal(data, &clashConfig); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidClashConfig, err)
	}

	var nodes []Node
	for _, proxy := range clashConfig.Proxies {
		node := cp.convertClashToSingBox(proxy)
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// convertClashToSingBox converts a Clash proxy configuration to SingBox format.
// It handles different proxy types (shadowsocks, vmess, vless, trojan) and
// converts their specific configurations to SingBox compatible format.
func (cp *ClashProcessor) convertClashToSingBox(proxy ClashProxy) Node {
	node := Node{
		"type":   strings.ToLower(proxy.Type),
		"tag":    proxy.Name,
		"server": proxy.Server,
	}

	// 转换端口
	port, err := strconv.Atoi(proxy.Port)
	if err != nil {
		log.Printf("%v for proxy %s: %v", ErrPortConversionFailed, proxy.Name, err)
		return nil
	}
	node["server_port"] = port

	// 处理UDP相关的packet_encoding
	if proxy.UDP {
		if proxy.Type == "vmess" || proxy.Type == "vless" {
			node["packet_encoding"] = "xudp"
		}
	}

	// 处理WebSocket传输配置
	if proxy.Network == "ws" {
		transport := map[string]any{
			"type": "ws",
			"path": proxy.WSPath,
		}
		if len(proxy.WSHeaders) > 0 {
			headers := make(map[string]any)
			for k, v := range proxy.WSHeaders {
				headers[k] = v
			}
			transport["headers"] = headers
		}
		node["transport"] = transport
	}

	// 处理TLS配置
	if proxy.TLS {
		tls := map[string]any{
			"enabled":     true,
			"server_name": proxy.ServerName,
			"insecure":    proxy.SkipCertVerify,
		}
		node["tls"] = tls
	}

	// 根据不同类型设置特定字段
	switch strings.ToLower(proxy.Type) {
	case "ss":
		node["type"] = "shadowsocks"
		node["method"] = proxy.Cipher
		node["password"] = proxy.Password

	case "vmess":
		node["type"] = "vmess"
		node["uuid"] = proxy.UUID
		if alterId, err := strconv.Atoi(proxy.AlterId); err == nil {
			node["alter_id"] = alterId
		}
		node["security"] = proxy.Cipher

	case "vless":
		node["type"] = "vless"
		node["uuid"] = proxy.UUID
		node["security"] = proxy.Cipher

	case "trojan":
		node["type"] = "trojan"
		node["password"] = proxy.Password

	default:
		log.Printf("%v: %s for proxy %s", ErrUnsupportedProxyType, proxy.Type, proxy.Name)
		return nil
	}

	return node
}
