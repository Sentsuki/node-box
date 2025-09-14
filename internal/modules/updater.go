// Package modules provides configuration file updating functionality for modules.
// It handles merging module configurations into target configuration files.
package modules

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"

	"node-box/internal/config"
)

// ConfigUpdater package errors
var (
	ErrConfigFileNotFound  = errors.New("config file not found")
	ErrConfigFileRead      = errors.New("failed to read config file")
	ErrConfigFileParse     = errors.New("failed to parse config file")
	ErrConfigFileWrite     = errors.New("failed to write config file")
	ErrModuleNotFound      = errors.New("module not found")
	ErrInvalidConfigFormat = errors.New("invalid config format")
)

// ConfigUpdater handles updating configuration files with module data.
// It can merge module configurations into target configuration files
// and supports JSON format.
type ConfigUpdater struct {
	moduleManager *ModuleManager
}

// NewConfigUpdater creates a new ConfigUpdater instance.
func NewConfigUpdater(moduleManager *ModuleManager) *ConfigUpdater {
	return &ConfigUpdater{
		moduleManager: moduleManager,
	}
}

// UpdateConfigFile updates a configuration file with the specified modules.
// It merges the module configurations into the target file based on the module types.
func (cu *ConfigUpdater) UpdateConfigFile(configFile config.ConfigFile) error {
	filePath := configFile.File

	log.Printf("开始更新配置文件: %s (%s)", configFile.Name, filePath)

	// Check if config file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrConfigFileNotFound, filePath)
	}

	// Read the target configuration file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileRead, filePath, err)
	}

	// Parse the configuration file as JSON
	var targetConfig map[string]any
	if err := json.Unmarshal(data, &targetConfig); err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileParse, filePath, err)
	}

	// Apply modules to the configuration
	updatedConfig, err := cu.applyModules(targetConfig, configFile.Modules)
	if err != nil {
		return fmt.Errorf("应用模块失败 %s: %v", configFile.Name, err)
	}

	// Write the updated configuration back to file
	if err := cu.writeConfigFile(filePath, updatedConfig); err != nil {
		return fmt.Errorf("写入配置文件失败 %s: %v", filePath, err)
	}

	log.Printf("成功更新配置文件: %s", filePath)
	return nil
}

// applyModules applies the specified modules to the target configuration.
// It merges module data into the appropriate sections of the configuration.
func (cu *ConfigUpdater) applyModules(targetConfig map[string]any, moduleNames []string) (map[string]any, error) {
	updatedConfig := make(map[string]any)

	// Copy the original configuration
	for k, v := range targetConfig {
		updatedConfig[k] = v
	}

	// Apply each module
	for _, moduleName := range moduleNames {
		moduleData, exists := cu.moduleManager.GetModule(moduleName)
		if !exists {
			return nil, fmt.Errorf("%w: %s", ErrModuleNotFound, moduleName)
		}

		// Determine module type and apply accordingly
		if err := cu.applyModuleByType(updatedConfig, moduleData, moduleName); err != nil {
			return nil, fmt.Errorf("应用模块 %s 失败: %v", moduleName, err)
		}
	}

	return updatedConfig, nil
}

// applyModuleByType applies a module by directly replacing/inserting the module data.
// Since remote modules are standard JSON, we directly replace/insert without type detection.
// This avoids element loss that could occur during complex parsing and mapping.
func (cu *ConfigUpdater) applyModuleByType(targetConfig map[string]any, moduleData map[string]any, moduleName string) error {
	log.Printf("应用模块 %s", moduleName)

	// 直接替换整个模块数据到目标配置中
	// 远程模块都是标准JSON，无需复杂解析，直接替换避免元素丢失
	for key, value := range moduleData {
		targetConfig[key] = value
		log.Printf("已应用配置段: %s", key)
	}

	return nil
}

// writeConfigFile writes the updated configuration to a file as JSON.
func (cu *ConfigUpdater) writeConfigFile(filePath string, config map[string]any) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConfigFileWrite, err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileWrite, filePath, err)
	}

	return nil
}
