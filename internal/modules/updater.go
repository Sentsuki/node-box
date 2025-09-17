// Package modules provides configuration file updating functionality for modules.
// It handles merging module configurations into target configuration files.
package modules

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

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
	filePath := configFile.Path

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

	// 1. 检查 endpoints 里是否有节点tag带有 [ ] ，即来自订阅的节点，如果有，清除掉
	if endpoints, ok := targetConfig["endpoints"].([]any); ok {
		var cleanedEndpoints []any
		for _, endpoint := range endpoints {
			if endpointMap, ok := endpoint.(map[string]any); ok {
				if tag, ok := endpointMap["tag"].(string); ok {
					// 检查tag是否包含 [ ] 格式，表示来自订阅
					if !cu.containsSubscriptionTag(tag) {
						cleanedEndpoints = append(cleanedEndpoints, endpoint)
					} else {
						log.Printf("清除来自订阅的endpoints节点: %s", tag)
					}
				} else {
					cleanedEndpoints = append(cleanedEndpoints, endpoint)
				}
			} else {
				cleanedEndpoints = append(cleanedEndpoints, endpoint)
			}
		}
		targetConfig["endpoints"] = cleanedEndpoints
		log.Printf("清理endpoints完成，保留 %d 个节点", len(cleanedEndpoints))
	}

	// 2. 检查 outbounds 的节点是否有这些 type: wireguard tailscale，如果有，移动到 endpoints 里
	if outbounds, ok := targetConfig["outbounds"].([]any); ok {
		var remainingOutbounds []any
		var movedToEndpoints []any

		for _, outbound := range outbounds {
			if outboundMap, ok := outbound.(map[string]any); ok {
				if outboundType, ok := outboundMap["type"].(string); ok {
					if outboundType == "wireguard" || outboundType == "tailscale" {
						movedToEndpoints = append(movedToEndpoints, outbound)
						if tag, ok := outboundMap["tag"].(string); ok {
							log.Printf("移动 %s 节点到endpoints: %s", outboundType, tag)
						}
					} else {
						remainingOutbounds = append(remainingOutbounds, outbound)
					}
				} else {
					remainingOutbounds = append(remainingOutbounds, outbound)
				}
			} else {
				remainingOutbounds = append(remainingOutbounds, outbound)
			}
		}

		// 更新 outbounds
		targetConfig["outbounds"] = remainingOutbounds

		// 将移动的节点添加到 endpoints
		if len(movedToEndpoints) > 0 {
			if existingEndpoints, ok := targetConfig["endpoints"].([]any); ok {
				targetConfig["endpoints"] = append(existingEndpoints, movedToEndpoints...)
			} else {
				targetConfig["endpoints"] = movedToEndpoints
			}
			log.Printf("移动 %d 个节点到endpoints", len(movedToEndpoints))
		}
	}

	// 3. 然后再执行合并逻辑
	// 直接替换整个模块数据到目标配置中
	// 远程模块都是标准JSON，无需复杂解析，直接替换避免元素丢失
	for key, value := range moduleData {
		targetConfig[key] = value
		log.Printf("已应用配置段: %s", key)
	}

	return nil
}

// containsSubscriptionTag 检查tag是否包含订阅标识符格式 [xxx]
func (cu *ConfigUpdater) containsSubscriptionTag(tag string) bool {
	// 检查是否包含 [ 和 ] 格式的订阅标识符
	return strings.Contains(tag, "[") && strings.Contains(tag, "]")
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
