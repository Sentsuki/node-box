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
					Name:   "示例订阅1",
					URL:    "https://example.com/clash-subscription",
					Type:   "clash",
					Enable: true,
				},
				{
					Name:   "示例订阅2",
					URL:    "https://example.com/singbox-subscription",
					Type:   "singbox",
					Enable: true,
				},
			},
			Targets: []ConfigPath{
				{
					InsertPath:   "./configs",
					InsertMarker: "🚀 节点选择",
				},
				{
					InsertPath:   "./test",
					InsertMarker: "Proxy",
				},
			},
			ExcludeKeywords: []string{"故障转移", "流量"},
		},
		Modules: &ModulesConfig{
			Log: []Module{
				{
					Name:     "log1",
					FromPath: "./configs/log.json",
				},
				{
					Name:    "log2",
					FromURL: "https://example.com/log.json",
				},
			},
			DNS: []Module{
				{
					Name:     "dns1",
					FromPath: "./configs/dns.json",
				},
				{
					Name:    "dns2",
					FromURL: "https://example.com/dns.json",
				},
			},
		},
		Configs: []ConfigFile{
			{
				Name:    "config1",
				Path:    "./singbox/my_config.json",
				Modules: []string{"log1", "dns1"},
			},
			{
				Name:    "config2",
				Path:    "./singbox/test_config.yaml",
				Modules: []string{"log2", "dns2"},
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
