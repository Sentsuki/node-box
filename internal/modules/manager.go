// Package modules provides module management functionality for configuration updates.
// It handles fetching modules from local paths or remote URLs and managing module data.
package modules

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"node-box/internal/client"
	"node-box/internal/config"
)

// ModuleManager package errors
var (
	ErrModuleManagerNotFound = errors.New("module not found")
	ErrModuleFetchFailed     = errors.New("failed to fetch module")
	ErrModuleParseFailed     = errors.New("failed to parse module data")
	ErrModuleTypeNotFound    = errors.New("module type not found")
	ErrInvalidModuleData     = errors.New("invalid module data")
)

// ModuleCache holds cached module data
type ModuleCache struct {
	modules map[string]map[string]any // module name -> module data
	valid   bool                      // 缓存是否有效
}

// ModuleManager handles fetching and managing configuration modules.
// It can fetch modules from local files or remote URLs and provides
// a unified interface for accessing module data.
type ModuleManager struct {
	config  *config.Config
	fetcher *client.Fetcher
	cache   *ModuleCache
}

// NewModuleManager creates a new ModuleManager instance.
// It initializes the module manager with the provided configuration and HTTP client.
func NewModuleManager(cfg *config.Config, fetcher *client.Fetcher) *ModuleManager {
	return &ModuleManager{
		config:  cfg,
		fetcher: fetcher,
		cache: &ModuleCache{
			modules: make(map[string]map[string]any),
			valid:   false,
		},
	}
}

// InvalidateCache invalidates the module cache, forcing a fresh fetch on next request.
func (mm *ModuleManager) InvalidateCache() {
	mm.cache.valid = false
	mm.cache.modules = make(map[string]map[string]any)
	log.Println("模块缓存已失效")
}

// FetchAllModules fetches all configured modules from their sources and caches them.
// It processes both local file modules and remote URL modules,
// returning any errors encountered during the process.
// Uses cached data if available and valid.
func (mm *ModuleManager) FetchAllModules() error {
	// 如果缓存有效，直接返回
	if mm.cache.valid {
		log.Printf("使用缓存的模块数据，共 %d 个模块", len(mm.cache.modules))
		return nil
	}

	if mm.config.Modules == nil {
		log.Println("No modules configured, skipping module fetch")
		return nil
	}

	log.Println("开始获取并缓存所有模块...")

	// 清空缓存
	mm.cache.modules = make(map[string]map[string]any)
	mm.cache.valid = false

	var fetchErrors []string
	successCount := 0

	// Fetch log modules
	for _, module := range mm.config.Modules.Log {
		if err := mm.fetchModule(module, "log"); err != nil {
			errorMsg := fmt.Sprintf("获取日志模块失败 %s: %v", module.Name, err)
			log.Printf("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}
		successCount++
	}

	// Fetch DNS modules
	for _, module := range mm.config.Modules.DNS {
		if err := mm.fetchModule(module, "dns"); err != nil {
			errorMsg := fmt.Sprintf("获取DNS模块失败 %s: %v", module.Name, err)
			log.Printf("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}
		successCount++
	}

	// Fetch NTP modules
	for _, module := range mm.config.Modules.NTP {
		if err := mm.fetchModule(module, "ntp"); err != nil {
			errorMsg := fmt.Sprintf("获取NTP模块失败 %s: %v", module.Name, err)
			log.Printf("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}
		successCount++
	}

	// Fetch Certificate modules
	for _, module := range mm.config.Modules.Certificate {
		if err := mm.fetchModule(module, "certificate"); err != nil {
			errorMsg := fmt.Sprintf("获取Certificate模块失败 %s: %v", module.Name, err)
			log.Printf("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}
		successCount++
	}

	// Fetch Endpoints modules
	for _, module := range mm.config.Modules.Endpoints {
		if err := mm.fetchModule(module, "endpoints"); err != nil {
			errorMsg := fmt.Sprintf("获取Endpoints模块失败 %s: %v", module.Name, err)
			log.Printf("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}
		successCount++
	}

	// Fetch Inbounds modules
	for _, module := range mm.config.Modules.Inbounds {
		if err := mm.fetchModule(module, "inbounds"); err != nil {
			errorMsg := fmt.Sprintf("获取Inbounds模块失败 %s: %v", module.Name, err)
			log.Printf("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}
		successCount++
	}

	// Fetch Outbounds modules
	for _, module := range mm.config.Modules.Outbounds {
		if err := mm.fetchModule(module, "outbounds"); err != nil {
			errorMsg := fmt.Sprintf("获取Outbounds模块失败 %s: %v", module.Name, err)
			log.Printf("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}
		successCount++
	}

	// Fetch Route modules
	for _, module := range mm.config.Modules.Route {
		if err := mm.fetchModule(module, "route"); err != nil {
			errorMsg := fmt.Sprintf("获取Route模块失败 %s: %v", module.Name, err)
			log.Printf("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}
		successCount++
	}

	// Fetch Services modules
	for _, module := range mm.config.Modules.Services {
		if err := mm.fetchModule(module, "services"); err != nil {
			errorMsg := fmt.Sprintf("获取Services模块失败 %s: %v", module.Name, err)
			log.Printf("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}
		successCount++
	}

	// Fetch Experimental modules
	for _, module := range mm.config.Modules.Experimental {
		if err := mm.fetchModule(module, "experimental"); err != nil {
			errorMsg := fmt.Sprintf("获取Experimental模块失败 %s: %v", module.Name, err)
			log.Printf("%s", errorMsg)
			fetchErrors = append(fetchErrors, errorMsg)
			continue
		}
		successCount++
	}

	// 标记缓存为有效（即使有部分失败）
	if successCount > 0 {
		mm.cache.valid = true
		log.Printf("模块缓存完成: 成功 %d 个，失败 %d 个", successCount, len(fetchErrors))
	}

	if len(fetchErrors) > 0 {
		log.Println("获取失败的模块:")
		for _, errMsg := range fetchErrors {
			log.Printf("  - %s", errMsg)
		}

		if successCount == 0 {
			return fmt.Errorf("所有模块获取失败: %v", fetchErrors)
		}

		return fmt.Errorf("部分模块获取失败: %d 成功, %d 失败", successCount, len(fetchErrors))
	}

	log.Printf("所有模块缓存成功，总计 %d 个模块", successCount)
	return nil
}

// fetchModule fetches a single module from its configured source with retry support.
// It handles both local file paths and remote URLs.
// Remote modules are expected to be standard JSON format.
// For remote URLs, it uses the same retry mechanism as subscription fetching (3 retries with delay).
func (mm *ModuleManager) fetchModule(module config.Module, moduleType string) error {
	var data []byte
	var err error

	if module.Path != "" {
		// Fetch from local file (no retry needed for local files)
		data, err = mm.fetchFromPath(module.Path)
		if err != nil {
			return fmt.Errorf("%w %s: %v", ErrModuleFetchFailed, module.Name, err)
		}
	} else if module.FromURL != "" {
		// Fetch from remote URL with retry support (uses fetcher's built-in retry mechanism)
		log.Printf("获取远程模块: %s (%s) from %s", module.Name, moduleType, module.FromURL)
		data, err = mm.fetcher.FetchSubscription(module.FromURL)
		if err != nil {
			return fmt.Errorf("%w %s: %v", ErrModuleFetchFailed, module.Name, err)
		}
	} else {
		return fmt.Errorf("%w %s: no source specified", ErrModuleFetchFailed, module.Name)
	}

	// Parse module data as standard JSON
	var moduleData map[string]any
	if err := json.Unmarshal(data, &moduleData); err != nil {
		return fmt.Errorf("%w %s: %v", ErrModuleParseFailed, module.Name, err)
	}

	// Store module data in cache
	mm.cache.modules[module.Name] = moduleData
	log.Printf("成功获取模块 %s (%s)", module.Name, moduleType)

	return nil
}

// fetchFromPath reads a module from a local file path with retry support.
// It handles both absolute and relative paths.
// Retries up to 3 times for transient file access issues (e.g., file locks).
func (mm *ModuleManager) fetchFromPath(path string) ([]byte, error) {
	// Convert relative path to absolute path if needed
	if !filepath.IsAbs(path) {
		path = filepath.Join(".", path)
	}

	const maxRetries = 3
	const retryDelay = 500 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("第 %d 次重试读取模块文件 %s，等待 %v...", attempt, path, retryDelay)
			time.Sleep(retryDelay)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				// File not found is not a transient error, don't retry
				return nil, fmt.Errorf("module file not found: %s", path)
			}

			lastErr = err
			log.Printf("读取模块文件失败 (尝试 %d/%d): %s - %v", attempt+1, maxRetries+1, path, err)
			continue
		}

		log.Printf("成功读取模块文件: %s (%d 字节)", path, len(data))
		return data, nil
	}

	return nil, fmt.Errorf("读取模块文件失败，已重试 %d 次: %s - %v", maxRetries, path, lastErr)
}

// GetModule retrieves a module by name from cache.
// It returns the module data and a boolean indicating if the module was found.
func (mm *ModuleManager) GetModule(name string) (map[string]any, bool) {
	module, exists := mm.cache.modules[name]
	return module, exists
}

// GetModulesByType retrieves all modules of a specific type.
// It returns a map of module names to their data.
func (mm *ModuleManager) GetModulesByType(moduleType string) map[string]map[string]any {
	result := make(map[string]map[string]any)

	if mm.config.Modules == nil {
		return result
	}

	var modules []config.Module
	switch moduleType {
	case "log":
		modules = mm.config.Modules.Log
	case "dns":
		modules = mm.config.Modules.DNS
	case "ntp":
		modules = mm.config.Modules.NTP
	case "certificate":
		modules = mm.config.Modules.Certificate
	case "endpoints":
		modules = mm.config.Modules.Endpoints
	case "inbounds":
		modules = mm.config.Modules.Inbounds
	case "outbounds":
		modules = mm.config.Modules.Outbounds
	case "route":
		modules = mm.config.Modules.Route
	case "services":
		modules = mm.config.Modules.Services
	case "experimental":
		modules = mm.config.Modules.Experimental
	default:
		log.Printf("未知的模块类型: %s", moduleType)
		return result
	}

	for _, module := range modules {
		if moduleData, exists := mm.cache.modules[module.Name]; exists {
			result[module.Name] = moduleData
		}
	}

	return result
}

// ListModules returns a list of all available module names from cache.
func (mm *ModuleManager) ListModules() []string {
	var names []string
	for name := range mm.cache.modules {
		names = append(names, name)
	}
	return names
}

// HasModule checks if a module exists in cache.
func (mm *ModuleManager) HasModule(name string) bool {
	_, exists := mm.cache.modules[name]
	return exists
}
