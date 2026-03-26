// Package modules provides module management functionality for configuration updates.
// It handles fetching modules from local paths or remote URLs and managing module data.
package modules

import (
	"encoding/json"
	"errors"
	"fmt"

	"node-box/internal/client"
	"node-box/internal/config"
	"node-box/internal/logger"
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
	logger.Debug("模块缓存已失效")
}

// ClearCache completely clears all cached module data and resets cache state.
// This method should be called after completing all module operations to free memory.
func (mm *ModuleManager) ClearCache() {
	mm.cache.valid = false
	mm.cache.modules = nil
	mm.cache = &ModuleCache{
		modules: make(map[string]map[string]any),
		valid:   false,
	}
	logger.Debug("所有模块缓存已清除")
}

// FetchAllModules fetches all configured modules from their sources and caches them.
// It processes both local file modules and remote URL modules,
// returning any errors encountered during the process.
// Uses cached data if available and valid.
func (mm *ModuleManager) FetchAllModules() error {
	// 如果缓存有效，直接返回
	if mm.cache.valid {
		logger.Debug("使用缓存的模块数据，共 %d 个模块", len(mm.cache.modules))
		return nil
	}

	if mm.config.Modules == nil {
		logger.Debug("No modules configured, skipping module fetch")
		return nil
	}

	logger.Debug("开始获取并缓存所有模块...")

	// 清空缓存
	mm.cache.modules = make(map[string]map[string]any)
	mm.cache.valid = false

	var fetchErrors []string
	successCount := 0

	// Fetch all module types using ModuleEntries
	for _, entry := range mm.config.Modules.ModuleEntries() {
		for _, module := range entry.Modules {
			if err := mm.fetchModule(module, entry.Type); err != nil {
				errorMsg := fmt.Sprintf("获取%s模块失败 %s: %v", entry.Type, module.Name, err)
				logger.Error("%s", errorMsg)
				fetchErrors = append(fetchErrors, errorMsg)
				continue
			}
			successCount++
		}
	}

	// 标记缓存为有效（即使有部分失败）
	if successCount > 0 {
		mm.cache.valid = true
		logger.Info("模块缓存完成: 成功 %d 个，失败 %d 个", successCount, len(fetchErrors))
	}

	if len(fetchErrors) > 0 {
		logger.Debug("获取失败的模块:")
		for _, errMsg := range fetchErrors {
			logger.Debug("  - %s", errMsg)
		}

		if successCount == 0 {
			logger.Warn("所有模块获取失败，但继续处理")
			return nil // 不返回错误，允许继续处理
		}

		logger.Warn("部分模块获取失败，但继续处理成功的 %d 个模块", successCount)
		return nil // 不返回错误，允许继续处理
	}

	logger.Debug("所有模块缓存成功，总计 %d 个模块", successCount)
	return nil
}

// fetchModule fetches a single module from its configured source with retry support.
// It handles both local file paths and remote URLs using the client fetcher.
// Remote modules are expected to be standard JSON format.
// Both local and remote fetching use the same retry mechanism for consistency.
func (mm *ModuleManager) fetchModule(module config.Module, moduleType string) error {
	var data []byte
	var err error

	if module.Path != "" {
		// Fetch from local file using client fetcher for consistency
		logger.Debug("获取本地模块: %s (%s) from %s", module.Name, moduleType, module.Path)
		data, err = mm.fetcher.FetchModuleFromPath(module.Path)
		if err != nil {
			return fmt.Errorf("%w %s: %v", ErrModuleFetchFailed, module.Name, err)
		}
	} else if module.FromURL != "" {
		// Fetch from remote URL with retry support (uses fetcher's built-in retry mechanism)
		logger.Debug("获取远程模块: %s (%s) from %s", module.Name, moduleType, module.FromURL)
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
	logger.Debug("成功获取模块 %s (%s)", module.Name, moduleType)

	return nil
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
	if mm.config.Modules != nil {
		modules = mm.config.Modules.ModulesByType(moduleType)
	}
	if modules == nil {
		logger.Warn("未知的模块类型: %s", moduleType)
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
