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

// ModuleManager handles fetching and managing configuration modules.
// It can fetch modules from local files or remote URLs and provides
// a unified interface for accessing module data.
type ModuleManager struct {
	config  *config.Config
	fetcher *client.Fetcher
	modules map[string]map[string]any // module name -> module data
}

// NewModuleManager creates a new ModuleManager instance.
// It initializes the module manager with the provided configuration and HTTP client.
func NewModuleManager(cfg *config.Config, fetcher *client.Fetcher) *ModuleManager {
	return &ModuleManager{
		config:  cfg,
		fetcher: fetcher,
		modules: make(map[string]map[string]any),
	}
}

// FetchAllModules fetches all configured modules from their sources.
// It processes both local file modules and remote URL modules,
// returning any errors encountered during the process.
func (mm *ModuleManager) FetchAllModules() error {
	if mm.config.Modules == nil {
		log.Println("No modules configured, skipping module fetch")
		return nil
	}

	log.Println("开始获取所有模块...")

	// Fetch log modules
	for _, module := range mm.config.Modules.Log {
		if err := mm.fetchModule(module, "log"); err != nil {
			log.Printf("获取日志模块失败 %s: %v", module.Name, err)
			continue
		}
	}

	// Fetch DNS modules
	for _, module := range mm.config.Modules.DNS {
		if err := mm.fetchModule(module, "dns"); err != nil {
			log.Printf("获取DNS模块失败 %s: %v", module.Name, err)
			continue
		}
	}

	// Fetch NTP modules
	for _, module := range mm.config.Modules.NTP {
		if err := mm.fetchModule(module, "ntp"); err != nil {
			log.Printf("获取NTP模块失败 %s: %v", module.Name, err)
			continue
		}
	}

	// Fetch Certificate modules
	for _, module := range mm.config.Modules.Certificate {
		if err := mm.fetchModule(module, "certificate"); err != nil {
			log.Printf("获取Certificate模块失败 %s: %v", module.Name, err)
			continue
		}
	}

	// Fetch Endpoints modules
	for _, module := range mm.config.Modules.Endpoints {
		if err := mm.fetchModule(module, "endpoints"); err != nil {
			log.Printf("获取Endpoints模块失败 %s: %v", module.Name, err)
			continue
		}
	}

	// Fetch Inbounds modules
	for _, module := range mm.config.Modules.Inbounds {
		if err := mm.fetchModule(module, "inbounds"); err != nil {
			log.Printf("获取Inbounds模块失败 %s: %v", module.Name, err)
			continue
		}
	}

	// Fetch Outbounds modules
	for _, module := range mm.config.Modules.Outbounds {
		if err := mm.fetchModule(module, "outbounds"); err != nil {
			log.Printf("获取Outbounds模块失败 %s: %v", module.Name, err)
			continue
		}
	}

	// Fetch Route modules
	for _, module := range mm.config.Modules.Route {
		if err := mm.fetchModule(module, "route"); err != nil {
			log.Printf("获取Route模块失败 %s: %v", module.Name, err)
			continue
		}
	}

	// Fetch Services modules
	for _, module := range mm.config.Modules.Services {
		if err := mm.fetchModule(module, "services"); err != nil {
			log.Printf("获取Services模块失败 %s: %v", module.Name, err)
			continue
		}
	}

	// Fetch Experimental modules
	for _, module := range mm.config.Modules.Experimental {
		if err := mm.fetchModule(module, "experimental"); err != nil {
			log.Printf("获取Experimental模块失败 %s: %v", module.Name, err)
			continue
		}
	}

	log.Printf("模块获取完成，共获取 %d 个模块", len(mm.modules))
	return nil
}

// fetchModule fetches a single module from its configured source.
// It handles both local file paths and remote URLs.
// Remote modules are expected to be standard JSON format.
func (mm *ModuleManager) fetchModule(module config.Module, moduleType string) error {
	var data []byte
	var err error

	if module.FromPath != "" {
		// Fetch from local file
		data, err = mm.fetchFromPath(module.FromPath)
		if err != nil {
			return fmt.Errorf("%w %s: %v", ErrModuleFetchFailed, module.Name, err)
		}
	} else if module.FromURL != "" {
		// Fetch from remote URL (expected to be standard JSON)
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

	// Store module data
	mm.modules[module.Name] = moduleData
	log.Printf("成功获取模块 %s (%s)", module.Name, moduleType)

	return nil
}

// fetchFromPath reads a module from a local file path.
// It handles both absolute and relative paths.
func (mm *ModuleManager) fetchFromPath(path string) ([]byte, error) {
	// Convert relative path to absolute path if needed
	if !filepath.IsAbs(path) {
		path = filepath.Join(".", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("module file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to read module file %s: %v", path, err)
	}

	return data, nil
}

// GetModule retrieves a module by name.
// It returns the module data and a boolean indicating if the module was found.
func (mm *ModuleManager) GetModule(name string) (map[string]any, bool) {
	module, exists := mm.modules[name]
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
		if moduleData, exists := mm.modules[module.Name]; exists {
			result[module.Name] = moduleData
		}
	}

	return result
}

// ListModules returns a list of all available module names.
func (mm *ModuleManager) ListModules() []string {
	var names []string
	for name := range mm.modules {
		names = append(names, name)
	}
	return names
}

// HasModule checks if a module exists.
func (mm *ModuleManager) HasModule(name string) bool {
	_, exists := mm.modules[name]
	return exists
}
