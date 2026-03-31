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

	"node-box/internal/logger"
)

// Config represents the main application configuration structure.
// It contains all settings needed for the node-box application to operate,
// including subscription sources, directory paths, and update intervals.
type Config struct {
	Nodes          *NodesConfig    `json:"nodes"`
	Modules        *ModulesConfig  `json:"modules,omitempty"`
	Configs        []ConfigFile    `json:"configs,omitempty"`
	UpdateSchedule *ScheduleConfig `json:"update_schedule"` // 调度配置
	Proxy          *ProxyConfig    `json:"proxy,omitempty"`
	UserAgent      string          `json:"user_agent,omitempty"`
	LogLevel       string          `json:"log_level,omitempty"` // 日志级别: silent, error, warn, info, debug
}

// NodesConfig represents the nodes configuration section.
// It contains subscriptions and exclude keywords.
type NodesConfig struct {
	Subscriptions   []Subscription     `json:"subscriptions"`
	ExcludeKeywords []string           `json:"exclude_keywords,omitempty"`
	RelayNodes      []IncludeRelayRule `json:"relay_nodes,omitempty"` // 确定哪些节点作为真实节点写入
}

// IncludeRelayRule defines a rule to include relay nodes whose tag contains
// the rule Tag and any of the Upstream keywords.
type IncludeRelayRule struct {
	Tag      string   `json:"tag"`
	Upstream []string `json:"upstream"`
}

// Subscription represents a single subscription source configuration.
// It defines the properties of a subscription including its URL or local path, type,
// and whether it's enabled for processing.
type Subscription struct {
	Name           string   `json:"name"`
	URL            string   `json:"url,omitempty"`  // 远程订阅URL，与Path二选一
	Path           string   `json:"path,omitempty"` // 本地文件路径，与URL二选一
	Type           string   `json:"type"`           // "clash", "singbox", "relay", "xray" or "v2ray"
	Enable         bool     `json:"enable"`
	Emoji          *bool    `json:"emoji,omitempty"`           // nil:保留原格式 true:根据节点名自动适配emoji false:移除emoji
	RemoveKeywords []string `json:"remove_keywords,omitempty"` // 从节点名称中移除的关键词列表
	UserAgent      string   `json:"user_agent,omitempty"`      // 自定义User-Agent，可选
}

// ModulesConfig represents the modules configuration section.
// It contains different types of modules that can be fetched from remote sources.
type ModulesConfig struct {
	Log                  []Module `json:"log,omitempty"`
	DNS                  []Module `json:"dns,omitempty"`
	NTP                  []Module `json:"ntp,omitempty"`
	Certificate          []Module `json:"certificate,omitempty"`
	CertificateProviders []Module `json:"certificate_providers,omitempty"`
	Endpoints            []Module `json:"endpoints,omitempty"`
	Inbounds             []Module `json:"inbounds,omitempty"`
	Outbounds            []Module `json:"outbounds,omitempty"`
	Route                []Module `json:"route,omitempty"`
	Services             []Module `json:"services,omitempty"`
	Experimental         []Module `json:"experimental,omitempty"`
}

// ModuleEntry represents a named group of modules for iteration.
type ModuleEntry struct {
	Type    string
	Modules []Module
}

// ModuleEntries returns all module groups with their type names in a fixed order.
// This eliminates repetitive per-field iteration across the codebase.
func (m *ModulesConfig) ModuleEntries() []ModuleEntry {
	return []ModuleEntry{
		{"log", m.Log},
		{"dns", m.DNS},
		{"ntp", m.NTP},
		{"certificate", m.Certificate},
		{"certificate_providers", m.CertificateProviders},
		{"endpoints", m.Endpoints},
		{"inbounds", m.Inbounds},
		{"outbounds", m.Outbounds},
		{"route", m.Route},
		{"services", m.Services},
		{"experimental", m.Experimental},
	}
}

// AllModuleNames returns a set of all module names across every type.
func (m *ModulesConfig) AllModuleNames() map[string]bool {
	names := make(map[string]bool)
	for _, entry := range m.ModuleEntries() {
		for _, module := range entry.Modules {
			names[module.Name] = true
		}
	}
	return names
}

// ModulesByType returns the module slice for the given type string, or nil if unknown.
func (m *ModulesConfig) ModulesByType(moduleType string) []Module {
	for _, entry := range m.ModuleEntries() {
		if entry.Type == moduleType {
			return entry.Modules
		}
	}
	return nil
}

// Selector replaces ProxyTarget under outbounds modules
type Selector struct {
	InsertMarker      string   `json:"insert_marker"`
	IncludeNodes      []string `json:"include_nodes,omitempty"`
	ExcludeNodes      []string `json:"exclude_nodes,omitempty"`
	IncludeRelayNodes []string `json:"include_relay_nodes,omitempty"` // 确定更新哪些 tag 到 selector
}

// Module represents a single module configuration.
// It defines how to fetch a module from a local path or remote URL.
type Module struct {
	Name          string     `json:"name"`               // module name
	Path          string     `json:"path,omitempty"`     // local file path
	FromURL       string     `json:"from_url,omitempty"` // remote URL
	Subscriptions []string   `json:"subscriptions,omitempty"`
	Selectors     []Selector `json:"selectors,omitempty"`
}

// ConfigFile represents a configuration file that uses modules.
// It defines which modules should be applied to which configuration file.
type ConfigFile struct {
	Name        string   `json:"name"`                    // configuration name
	Path        string   `json:"path"`                    // target configuration file path
	Modules     []string `json:"modules"`                 // list of module names to apply
	NoNeedNodes []string `json:"no_need_nodes,omitempty"` // keywords to remove from outbounds and endpoints after processing
}

// ScheduleConfig represents update schedule configuration.
// It allows users to choose between interval-based updates and hourly updates.
type ScheduleConfig struct {
	Type     string `json:"type"`               // "interval" or "hourly"
	Interval int    `json:"interval,omitempty"` // 更新间隔（小时），仅在 type 为 "interval" 时使用
	// 当 Type 为 "hourly" 时，程序会在每个整点执行更新
	// 当 Type 为 "interval" 时，使用 Interval 字段指定的小时数
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

// ConfigPathEnvVar defines the environment variable name for the custom configuration file path.
const ConfigPathEnvVar = "NODE_BOX_CONFIG"

// GetConfigPath returns the configuration file path.
// It first checks if a path is explicitly provided, then checks the environment variable,
// and finally falls back to the default path.
func GetConfigPath(providedPath, defaultPath string) string {
	// 如果明确提供了路径，使用提供的路径
	if providedPath != "" && providedPath != defaultPath {
		return providedPath
	}

	// 检查环境变量
	if envPath := os.Getenv(ConfigPathEnvVar); envPath != "" {
		return envPath
	}

	// 使用默认路径
	return defaultPath
}

// LoadFromPath reads and parses a configuration file from the specified path.
// It returns a Config struct populated with the configuration data,
// or an error if the file cannot be read or parsed.
func LoadFromPath(path string) (*Config, error) {
	return Load(path)
}

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

	// Log proxy configuration information (using standard log for now since logger may not be initialized)
	if config.Proxy != nil {
		logger.Debug("Proxy configuration: %s://%s:%d", config.Proxy.Type, config.Proxy.Host, config.Proxy.Port)
		if config.Proxy.Username != "" {
			logger.Info("Proxy authentication: %s", config.Proxy.Username)
		}
	} else {
		logger.Info("No proxy configured, using direct connection")
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

	// 验证调度配置
	if c.UpdateSchedule == nil {
		return fmt.Errorf("update_schedule configuration is required")
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

	// Validate schedule configuration
	if err := c.validateScheduleConfig(c.UpdateSchedule); err != nil {
		return err
	}

	return nil
}

// validateConfig validations

// validateSubscription validates a single subscription configuration
func (c *Config) validateSubscription(sub Subscription, index int) error {
	if sub.Name == "" {
		return fmt.Errorf("subscription %d: name cannot be empty", index)
	}

	// URL和Path必须有且仅有一个
	hasURL := sub.URL != ""
	hasPath := sub.Path != ""

	if !hasURL && !hasPath {
		return fmt.Errorf("subscription %d (%s): either URL or Path must be provided", index, sub.Name)
	}

	if hasURL && hasPath {
		return fmt.Errorf("subscription %d (%s): cannot specify both URL and Path", index, sub.Name)
	}

	validTypes := []string{"clash", "singbox", "relay", "xray", "v2ray"}
	subType := strings.ToLower(sub.Type)
	if !slices.Contains(validTypes, subType) {
		return fmt.Errorf("subscription %d (%s): invalid type '%s', must be one of: %v",
			index, sub.Name, sub.Type, validTypes)
	}

	return nil
}

// validateModulesConfig validates modules configuration
func (c *Config) validateModulesConfig(modules *ModulesConfig) error {
	for _, entry := range modules.ModuleEntries() {
		for i, module := range entry.Modules {
			if err := c.validateModule(module, entry.Type, i); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateModule validates a single module configuration
func (c *Config) validateModule(module Module, moduleType string, index int) error {
	if module.Name == "" {
		return fmt.Errorf("modules.%s[%d]: name cannot be empty", moduleType, index)
	}

	// Either path or from_url must be provided, but not both
	hasPath := module.Path != ""
	hasURL := module.FromURL != ""

	if !hasPath && !hasURL {
		return fmt.Errorf("modules.%s[%d] (%s): either path or from_url must be provided", moduleType, index, module.Name)
	}

	if hasPath && hasURL {
		return fmt.Errorf("modules.%s[%d] (%s): cannot specify both path and from_url", moduleType, index, module.Name)
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
		allModules := c.Modules.AllModuleNames()

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

// validateScheduleConfig validates schedule configuration
func (c *Config) validateScheduleConfig(schedule *ScheduleConfig) error {
	validTypes := []string{"interval", "hourly"}
	scheduleType := strings.ToLower(schedule.Type)
	if !slices.Contains(validTypes, scheduleType) {
		return fmt.Errorf("invalid schedule type '%s', must be one of: %v", schedule.Type, validTypes)
	}

	// 如果是间隔模式，必须指定间隔时间
	if scheduleType == "interval" && schedule.Interval <= 0 {
		return fmt.Errorf("interval must be greater than 0 when schedule type is 'interval'")
	}

	// 如果是整点模式，间隔时间应该为空或被忽略
	if scheduleType == "hourly" && schedule.Interval > 0 {
		logger.Info("Warning: interval field is ignored when schedule type is 'hourly'")
	}

	return nil
}
