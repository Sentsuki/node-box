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
	Nodes          *NodesConfig   `json:"nodes"`
	Modules        *ModulesConfig `json:"modules,omitempty"`
	Configs        []ConfigFile   `json:"configs,omitempty"`
	UpdateInterval int            `json:"update_interval_hours"`
	Proxy          *ProxyConfig   `json:"proxy,omitempty"`
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

// ModulesConfig represents the modules configuration section.
// It contains different types of modules that can be fetched from remote sources.
type ModulesConfig struct {
	Log []Module `json:"log,omitempty"`
	DNS []Module `json:"dns,omitempty"`
}

// Module represents a single module configuration.
// It defines how to fetch a module from a local path or remote URL.
type Module struct {
	Name     string `json:"name"`                // module name
	FromPath string `json:"from_path,omitempty"` // local file path
	FromURL  string `json:"from_url,omitempty"`  // remote URL
}

// ConfigFile represents a configuration file that uses modules.
// It defines which modules should be applied to which configuration file.
type ConfigFile struct {
	Name    string   `json:"name"`    // configuration name
	Path    string   `json:"path"`    // target configuration file path
	Modules []string `json:"modules"` // list of module names to apply
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

	// Validate modules configuration if present
	if c.Modules != nil {
		if err := c.validateModulesConfig(c.Modules); err != nil {
			return err
		}
	}

	// Validate configs if present
	for i, configFile := range c.Configs {
		if err := c.validateConfigFile(configFile, i); err != nil {
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

// validateModulesConfig validates modules configuration
func (c *Config) validateModulesConfig(modules *ModulesConfig) error {
	// Validate log modules
	for i, module := range modules.Log {
		if err := c.validateModule(module, "log", i); err != nil {
			return err
		}
	}

	// Validate DNS modules
	for i, module := range modules.DNS {
		if err := c.validateModule(module, "dns", i); err != nil {
			return err
		}
	}

	return nil
}

// validateModule validates a single module configuration
func (c *Config) validateModule(module Module, moduleType string, index int) error {
	if module.Name == "" {
		return fmt.Errorf("modules.%s[%d]: name cannot be empty", moduleType, index)
	}

	// Either from_path or from_url must be provided, but not both
	hasPath := module.FromPath != ""
	hasURL := module.FromURL != ""

	if !hasPath && !hasURL {
		return fmt.Errorf("modules.%s[%d] (%s): either from_path or from_url must be provided", moduleType, index, module.Name)
	}

	if hasPath && hasURL {
		return fmt.Errorf("modules.%s[%d] (%s): cannot specify both from_path and from_url", moduleType, index, module.Name)
	}

	return nil
}

// validateConfigFile validates a single config file configuration
func (c *Config) validateConfigFile(configFile ConfigFile, index int) error {
	if configFile.Name == "" {
		return fmt.Errorf("configs[%d]: name cannot be empty", index)
	}

	if configFile.Path == "" {
		return fmt.Errorf("configs[%d] (%s): path cannot be empty", index, configFile.Name)
	}

	if len(configFile.Modules) == 0 {
		return fmt.Errorf("configs[%d] (%s): modules cannot be empty", index, configFile.Name)
	}

	// Validate that all referenced modules exist
	if c.Modules != nil {
		allModules := make(map[string]bool)

		// Collect all module names
		for _, module := range c.Modules.Log {
			allModules[module.Name] = true
		}
		for _, module := range c.Modules.DNS {
			allModules[module.Name] = true
		}

		// Check if all referenced modules exist
		for _, moduleName := range configFile.Modules {
			if !allModules[moduleName] {
				return fmt.Errorf("configs[%d] (%s): module '%s' not found", index, configFile.Name, moduleName)
			}
		}
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
