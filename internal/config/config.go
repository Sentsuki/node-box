// Package config provides configuration management for the node-box application.
// It handles loading, validation, and management of application configuration
// including subscriptions, proxy settings, and update intervals.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
)

// Config represents the main application configuration structure.
// It contains all settings needed for the node-box application to operate,
// including subscription sources, directory paths, and update intervals.
type Config struct {
	Nodes          *NodesConfig `json:"nodes"`
	UpdateInterval int          `json:"update_interval_hours"`
	Proxy          *ProxyConfig `json:"proxy,omitempty"`
}

// NodesConfig represents the nodes configuration section.
// It contains subscriptions, targets, and exclude keywords.
type NodesConfig struct {
	Subscriptions   []Subscription `json:"subscriptions"`
	Targets         []ConfigPath   `json:"targets"`
	ExcludeKeywords []string       `json:"exclude_keywords,omitempty"`
}

// Subscription represents a single subscription source configuration.
// It defines the properties of a subscription including its URL, type,
// and whether it's enabled for processing.
type Subscription struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Type   string `json:"type"` // "clash" or "singbox"
	Enable bool   `json:"enable"`
}

// ConfigPath represents a configuration path with its associated insert marker.
// It defines where configuration files are located and which marker to use for updates.
type ConfigPath struct {
	InsertPath   string `json:"insert_path"`
	InsertMarker string `json:"insert_marker"`
}

// ProxyConfig represents proxy server configuration.
// It supports HTTP, HTTPS, and SOCKS5 proxy types with optional authentication.
type ProxyConfig struct {
	Type     string `json:"type"`     // "http", "https", "socks5"
	Host     string `json:"host"`     // proxy server address
	Port     int    `json:"port"`     // proxy server port
	Username string `json:"username"` // username (optional)
	Password string `json:"password"` // password (optional)
}

// Configuration errors
var (
	ErrConfigNotFound        = errors.New("config file not found")
	ErrInvalidConfigFormat   = errors.New("invalid config format")
	ErrProxyConfigInvalid    = errors.New("invalid proxy configuration")
	ErrInvalidUpdateInterval = errors.New("update interval must be greater than 0")
)

// Load reads and parses a configuration file from the specified path.
// It returns a Config struct populated with the configuration data,
// or an error if the file cannot be read or parsed.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, path)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfigFormat, err)
	}

	// Log proxy configuration information
	if config.Proxy != nil {
		log.Printf("Proxy configuration: %s://%s:%d", config.Proxy.Type, config.Proxy.Host, config.Proxy.Port)
		if config.Proxy.Username != "" {
			log.Printf("Proxy authentication: %s", config.Proxy.Username)
		}
	} else {
		log.Println("No proxy configured, using direct connection")
	}

	return &config, nil
}

// Validate checks if the configuration is valid and returns an error if not.
// It validates all required fields and ensures the configuration is consistent.
func (c *Config) Validate() error {
	// 验证 nodes 配置
	if c.Nodes == nil {
		return fmt.Errorf("nodes configuration is required")
	}

	// 验证 targets (配置路径)
	if len(c.Nodes.Targets) == 0 {
		return fmt.Errorf("nodes.targets cannot be empty")
	}

	for i, target := range c.Nodes.Targets {
		if err := c.validateConfigPath(target, i); err != nil {
			return err
		}
	}

	if c.UpdateInterval <= 0 {
		return ErrInvalidUpdateInterval
	}

	// Validate subscriptions
	for i, sub := range c.Nodes.Subscriptions {
		if err := c.validateSubscription(sub, i); err != nil {
			return err
		}
	}

	// Validate proxy configuration if present
	if c.Proxy != nil {
		if err := c.validateProxyConfig(c.Proxy); err != nil {
			return err
		}
	}

	return nil
}

// validateConfigPath validates a single config path configuration
func (c *Config) validateConfigPath(configPath ConfigPath, index int) error {
	if configPath.InsertPath == "" {
		return fmt.Errorf("targets[%d]: insert_path cannot be empty", index)
	}

	if configPath.InsertMarker == "" {
		return fmt.Errorf("targets[%d]: insert_marker cannot be empty", index)
	}

	return nil
}

// validateSubscription validates a single subscription configuration
func (c *Config) validateSubscription(sub Subscription, index int) error {
	if sub.Name == "" {
		return fmt.Errorf("subscription %d: name cannot be empty", index)
	}

	if sub.URL == "" {
		return fmt.Errorf("subscription %d (%s): URL cannot be empty", index, sub.Name)
	}

	validTypes := []string{"clash", "singbox"}
	subType := strings.ToLower(sub.Type)
	if !slices.Contains(validTypes, subType) {
		return fmt.Errorf("subscription %d (%s): invalid type '%s', must be one of: %v",
			index, sub.Name, sub.Type, validTypes)
	}

	return nil
}

// validateProxyConfig validates proxy configuration
func (c *Config) validateProxyConfig(proxy *ProxyConfig) error {
	if proxy.Host == "" {
		return fmt.Errorf("%w: host cannot be empty", ErrProxyConfigInvalid)
	}

	if proxy.Port <= 0 || proxy.Port > 65535 {
		return fmt.Errorf("%w: port must be between 1 and 65535", ErrProxyConfigInvalid)
	}

	validTypes := []string{"http", "https", "socks5"}
	proxyType := strings.ToLower(proxy.Type)
	if !slices.Contains(validTypes, proxyType) {
		return fmt.Errorf("%w: invalid type '%s', must be one of: %v",
			ErrProxyConfigInvalid, proxy.Type, validTypes)
	}

	// If username is provided, password should also be provided
	if proxy.Username != "" && proxy.Password == "" {
		log.Printf("Warning: proxy username provided but password is empty")
	}

	return nil
}
