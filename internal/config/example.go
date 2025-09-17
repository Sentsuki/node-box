package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// GenerateExample creates an example configuration file at the specified path.
// This function generates a sample configuration with typical settings that
// users can modify according to their needs. It includes example subscriptions,
// proxy configuration, and other common settings.
func GenerateExample(configPath string) error {
	config := Config{
		Nodes: &NodesConfig{
			Subscriptions: []Subscription{
				{
					Name:   "示例订阅1-远程",
					URL:    "https://example.com/clash-subscription",
					Type:   "clash",
					Enable: true,
				},
				{
					Name:   "示例订阅2-远程",
					URL:    "https://example.com/singbox-subscription",
					Type:   "singbox",
					Enable: true,
				},
				{
					Name:   "示例订阅3-本地",
					Path:   "./subscriptions/local-clash.yaml",
					Type:   "clash",
					Enable: true,
				},
				{
					Name:   "示例订阅4-本地",
					Path:   "./subscriptions/local-singbox.json",
					Type:   "singbox",
					Enable: false,
				},
			},
			Targets: []Target{
				{
					Name: "默认目录多规则示例",
					Path: "./configs",
					Proxies: []ProxyTarget{
						{InsertMarker: "🚀 节点选择"},
						{InsertMarker: "🌟 特定节点", IncludeKeywords: []string{"美国", "日本"}},
					},
				},
				{
					Name:          "单文件规则示例",
					Path:          "./test/specific_config.json",
					IsFile:        true,
					Subscriptions: []string{"示例订阅1"},
					Proxies: []ProxyTarget{
						{InsertMarker: "Proxy"},
					},
				},
			},
			ExcludeKeywords: []string{"故障转移", "流量"},
		},
		Modules: &ModulesConfig{
			Log: []Module{
				{
					Name: "log1",
					Path: "./configs/log.json",
				},
				{
					Name:    "log2",
					FromURL: "https://example.com/log.json",
				},
			},
			DNS: []Module{
				{
					Name: "dns1",
					Path: "./configs/dns.json",
				},
				{
					Name:    "dns2",
					FromURL: "https://example.com/dns.json",
				},
			},
			NTP: []Module{
				{
					Name: "ntp1",
					Path: "./configs/ntp.json",
				},
			},
			Certificate: []Module{
				{
					Name:    "cert1",
					FromURL: "https://example.com/certificate.json",
				},
			},
			Endpoints: []Module{
				{
					Name: "endpoints1",
					Path: "./configs/endpoints.json",
				},
			},
			Inbounds: []Module{
				{
					Name:    "inbounds1",
					FromURL: "https://example.com/inbounds.json",
				},
			},
			Outbounds: []Module{
				{
					Name: "outbounds1",
					Path: "./configs/outbounds.json",
				},
			},
			Route: []Module{
				{
					Name:    "route1",
					FromURL: "https://example.com/route.json",
				},
			},
			Services: []Module{
				{
					Name: "services1",
					Path: "./configs/services.json",
				},
			},
			Experimental: []Module{
				{
					Name:    "experimental1",
					FromURL: "https://example.com/experimental.json",
				},
			},
		},
		Configs: []ConfigFile{
			{
				Name:    "config1",
				Path:    "./singbox/my_config.json",
				Modules: []string{"log1", "dns1", "ntp1", "cert1"},
			},
			{
				Name:    "config2",
				Path:    "./singbox/test_config.json",
				Modules: []string{"log2", "dns2", "endpoints1", "inbounds1", "outbounds1", "route1", "services1", "experimental1"},
			},
		},
		UpdateInterval: 6,
		Proxy: &ProxyConfig{
			Type:     "http",
			Host:     "127.0.0.1",
			Port:     7890,
			Username: "username", // optional
			Password: "password", // optional
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal example config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write example config file: %w", err)
	}

	fmt.Printf("Generated example configuration file: %s\n", configPath)
	return nil
}
