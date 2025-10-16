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
// It contains subscriptions, targets, and exclude keywords.
type NodesConfig struct {
	Subscriptions   []Subscription     `json:"subscriptions"`
	Targets         []Target           `json:"targets"`
	ExcludeKeywords []string           `json:"exclude_keywords,omitempty"`
	IncludeRelay    []IncludeRelayRule `json:"include_relay,omitempty"` // 确定哪些节点作为真实节点写入
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
	Name      string `json:"name"`
	URL       string `json:"url,omitempty"`  // 远程订阅URL，与Path二选一
	Path      string `json:"path,omitempty"` // 本地文件路径，与URL二选一
	Type      string `json:"type"`           // "clash" or "singbox"
	Enable    bool   `json:"enable"`
	UserAgent string `json:"user_agent,omitempty"` // 自定义User-Agent，可选
}

// Target represents a configuration target path and a set of proxy insertion rules.
// A target may point to a directory or a single file, and can specify which
// subscriptions to include. Each target contains one or more proxy rules
// that define the insert marker and keyword filters.
type Target struct {
	Name          string        `json:"name,omitempty"`
	Path          string        `json:"path"`
	IsFile        bool          `json:"is_file,omitempty"`
	Subscriptions []string      `json:"subscriptions,omitempty"`
	Proxies       []ProxyTarget `json:"proxies"`
}

// ProxyTarget represents a single proxy insertion rule under a target.
// It defines the selector tag to insert into, and optional include/exclude
// keyword filters applied to node tags.
type ProxyTarget struct {
	InsertMarker    string   `json:"insert_marker"`
	IncludeKeywords []string `json:"include_keywords,omitempty"`
	ExcludeKeywords []string `json:"exclude_keywords,omitempty"`
	RelayNodes      []string `json:"relay_nodes,omitempty"` // 确定更新哪些 tag 到 selector
}

// ModulesConfig represents the modules configuration section.
// It contains different types of modules that can be fetched from remote sources.
type ModulesConfig struct {
	Log          []Module `json:"log,omitempty"`
	DNS          []Module `json:"dns,omitempty"`
	NTP          []Module `json:"ntp,omitempty"`
	Certificate  []Module `json:"certificate,omitempty"`
	Endpoints    []Module `json:"endpoints,omitempty"`
	Inbounds     []Module `json:"inbounds,omitempty"`
	Outbounds    []Module `json:"outbounds,omitempty"`
	Route        []Module `json:"route,omitempty"`
	Services     []Module `json:"services,omitempty"`
	Experimental []Module `json:"experimental,omitempty"`
}

// Module represents a single module configuration.
// It defines how to fetch a module from a local path or remote URL.
type Module struct {
	Name    string `json:"name"`               // module name
	Path    string `json:"path,omitempty"`     // local file path
	FromURL string `json:"from_url,omitempty"` // remote URL
}

// ConfigFile represents a configuration file that uses modules.
// It defines which modules should be applied to which configuration file.
type ConfigFile struct {
	Name    string   `json:"name"`              // configuration name
	Path    string   `json:"path"`              // target configuration file path
	Modules []string `json:"modules"`           // list of module names to apply
	NoNeed  []string `json:"no_need,omitempty"` // keywords to remove from outbounds and endpoints after processing
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

// Environment variable name for config file path
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

	// 验证 targets (配置路径)
	if len(c.Nodes.Targets) == 0 {
		return fmt.Errorf("nodes.targets cannot be empty")
	}

	for i, target := range c.Nodes.Targets {
		if err := c.validateTarget(target, i); err != nil {
			return err
		}
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

// validateTarget validates a single target configuration
func (c *Config) validateTarget(target Target, index int) error {
	if target.Path == "" {
		return fmt.Errorf("targets[%d]: path cannot be empty", index)
	}

	if len(target.Proxies) == 0 {
		return fmt.Errorf("targets[%d]: proxies cannot be empty", index)
	}

	// 验证指定的订阅是否存在
	if len(target.Subscriptions) > 0 {
		subscriptionMap := make(map[string]bool)
		for _, sub := range c.Nodes.Subscriptions {
			subscriptionMap[sub.Name] = true
		}

		for _, subName := range target.Subscriptions {
			if !subscriptionMap[subName] {
				return fmt.Errorf("targets[%d]: subscription '%s' not found in subscriptions list", index, subName)
			}
		}
	}

	// 验证每个 proxy 配置
	for j, p := range target.Proxies {
		if strings.TrimSpace(p.InsertMarker) == "" {
			return fmt.Errorf("targets[%d].proxies[%d]: insert_marker cannot be empty", index, j)
		}
		// include/exclude 关键词无需更多格式验证
	}

	return nil
}

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

	validTypes := []string{"clash", "singbox", "relay"}
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

	// Validate NTP modules
	for i, module := range modules.NTP {
		if err := c.validateModule(module, "ntp", i); err != nil {
			return err
		}
	}

	// Validate Certificate modules
	for i, module := range modules.Certificate {
		if err := c.validateModule(module, "certificate", i); err != nil {
			return err
		}
	}

	// Validate Endpoints modules
	for i, module := range modules.Endpoints {
		if err := c.validateModule(module, "endpoints", i); err != nil {
			return err
		}
	}

	// Validate Inbounds modules
	for i, module := range modules.Inbounds {
		if err := c.validateModule(module, "inbounds", i); err != nil {
			return err
		}
	}

	// Validate Outbounds modules
	for i, module := range modules.Outbounds {
		if err := c.validateModule(module, "outbounds", i); err != nil {
			return err
		}
	}

	// Validate Route modules
	for i, module := range modules.Route {
		if err := c.validateModule(module, "route", i); err != nil {
			return err
		}
	}

	// Validate Services modules
	for i, module := range modules.Services {
		if err := c.validateModule(module, "services", i); err != nil {
			return err
		}
	}

	// Validate Experimental modules
	for i, module := range modules.Experimental {
		if err := c.validateModule(module, "experimental", i); err != nil {
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
		allModules := make(map[string]bool)

		// Collect all module names from all module types
		for _, module := range c.Modules.Log {
			allModules[module.Name] = true
		}
		for _, module := range c.Modules.DNS {
			allModules[module.Name] = true
		}
		for _, module := range c.Modules.NTP {
			allModules[module.Name] = true
		}
		for _, module := range c.Modules.Certificate {
			allModules[module.Name] = true
		}
		for _, module := range c.Modules.Endpoints {
			allModules[module.Name] = true
		}
		for _, module := range c.Modules.Inbounds {
			allModules[module.Name] = true
		}
		for _, module := range c.Modules.Outbounds {
			allModules[module.Name] = true
		}
		for _, module := range c.Modules.Route {
			allModules[module.Name] = true
		}
		for _, module := range c.Modules.Services {
			allModules[module.Name] = true
		}
		for _, module := range c.Modules.Experimental {
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
