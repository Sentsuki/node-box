// Package modules provides configuration file updating functionality for modules.
// It handles merging module configurations into target configuration files.
package modules

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"node-box/internal/config"
	"node-box/internal/logger"
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

	logger.Debug("开始更新配置文件: %s (%s)", configFile.Name, filePath)

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

	// 在所有模块应用完成后执行后处理逻辑
	if err := cu.postProcessMergedConfig(updatedConfig); err != nil {
		return fmt.Errorf("后处理配置失败 %s: %v", configFile.Name, err)
	}

	// 执行 no_need 过滤逻辑
	if len(configFile.NoNeed) > 0 {
		if err := cu.applyNoNeedFilter(updatedConfig, configFile.NoNeed); err != nil {
			return fmt.Errorf("应用no_need过滤失败 %s: %v", configFile.Name, err)
		}
	}

	// Write the updated configuration back to file
	if err := cu.writeConfigFile(filePath, updatedConfig); err != nil {
		return fmt.Errorf("写入配置文件失败 %s: %v", filePath, err)
	}

	logger.Debug("成功更新配置文件: %s", filePath)
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
	logger.Debug("应用模块 %s", moduleName)

	// 直接替换整个模块数据到目标配置中
	// 远程模块都是标准JSON，无需复杂解析，直接替换避免元素丢失
	for key, value := range moduleData {
		targetConfig[key] = value
		logger.Debug("已应用配置段: %s", key)
	}

	return nil
}

// postProcessMergedConfig 在模块合并完成后执行后处理逻辑
func (cu *ConfigUpdater) postProcessMergedConfig(config map[string]any) error {
	logger.Debug("开始执行模块配置后处理...")

	// 1. 检查 endpoints 里是否有节点tag带有方括号[]，即来自订阅的节点，如果有，清除掉
	if err := cu.cleanSubscriptionNodesFromEndpoints(config); err != nil {
		logger.Error("清理endpoints中的订阅节点失败: %v", err)
		return err
	}

	// 2. 检查 outbounds 的节点是否有 type: wireguard, tailscale，如果有，移动到 endpoints 里
	if err := cu.moveSpecialOutboundsToEndpoints(config); err != nil {
		logger.Error("移动特殊outbounds到endpoints失败: %v", err)
		return err
	}

	logger.Info("模块处理完成")
	return nil
}

// cleanSubscriptionNodesFromEndpoints 清除 endpoints 中带有方括号[]的订阅节点
func (cu *ConfigUpdater) cleanSubscriptionNodesFromEndpoints(config map[string]any) error {
	endpoints, exists := config["endpoints"]
	if !exists {
		return nil
	}

	endpointsSlice, ok := endpoints.([]any)
	if !ok {
		return nil
	}

	var cleanedEndpoints []any
	removedCount := 0

	for _, endpoint := range endpointsSlice {
		endpointMap, ok := endpoint.(map[string]any)
		if !ok {
			cleanedEndpoints = append(cleanedEndpoints, endpoint)
			continue
		}

		// 检查tag字段是否包含方括号
		if tag, exists := endpointMap["tag"]; exists {
			if tagStr, ok := tag.(string); ok {
				// 如果tag包含方括号，说明是来自订阅的节点，需要清除
				if cu.containsSquareBrackets(tagStr) {
					logger.Debug("清除endpoints中的订阅节点: %s", tagStr)
					removedCount++
					continue
				}
			}
		}

		cleanedEndpoints = append(cleanedEndpoints, endpoint)
	}

	if removedCount > 0 {
		config["endpoints"] = cleanedEndpoints
		logger.Debug("从endpoints中清除了 %d 个订阅节点", removedCount)
	}

	return nil
}

// moveSpecialOutboundsToEndpoints 将 wireguard 和 tailscale 类型的 outbounds 移动到 endpoints
func (cu *ConfigUpdater) moveSpecialOutboundsToEndpoints(config map[string]any) error {
	outbounds, exists := config["outbounds"]
	if !exists {
		return nil
	}

	outboundsSlice, ok := outbounds.([]any)
	if !ok {
		return nil
	}

	var remainingOutbounds []any
	var movedEndpoints []any
	movedCount := 0

	for _, outbound := range outboundsSlice {
		outboundMap, ok := outbound.(map[string]any)
		if !ok {
			remainingOutbounds = append(remainingOutbounds, outbound)
			continue
		}

		// 检查type字段
		if outboundType, exists := outboundMap["type"]; exists {
			if typeStr, ok := outboundType.(string); ok {
				// 如果是 wireguard 或 tailscale 类型，移动到 endpoints
				if typeStr == "wireguard" || typeStr == "tailscale" {
					if tag, exists := outboundMap["tag"]; exists {
						if tagStr, ok := tag.(string); ok {
							logger.Debug("移动 %s 类型的节点到endpoints: %s", typeStr, tagStr)
						}
					}
					movedEndpoints = append(movedEndpoints, outbound)
					movedCount++
					continue
				}
			}
		}

		remainingOutbounds = append(remainingOutbounds, outbound)
	}

	if movedCount > 0 {
		// 更新 outbounds
		config["outbounds"] = remainingOutbounds

		// 更新 endpoints
		if endpoints, exists := config["endpoints"]; exists {
			if endpointsSlice, ok := endpoints.([]any); ok {
				config["endpoints"] = append(endpointsSlice, movedEndpoints...)
			} else {
				config["endpoints"] = movedEndpoints
			}
		} else {
			config["endpoints"] = movedEndpoints
		}

		logger.Debug("移动了 %d 个特殊类型节点从outbounds到endpoints", movedCount)
	}

	return nil
}

// containsSquareBrackets 检查字符串是否包含方括号
func (cu *ConfigUpdater) containsSquareBrackets(s string) bool {
	return strings.Contains(s, "[") && strings.Contains(s, "]")
}

// applyNoNeedFilter 根据 no_need 配置过滤 outbounds 和 endpoints 中包含关键词的节点
func (cu *ConfigUpdater) applyNoNeedFilter(config map[string]any, noNeedKeywords []string) error {
	if len(noNeedKeywords) == 0 {
		return nil
	}

	logger.Debug("开始应用no_need过滤，关键词: %v", noNeedKeywords)

	// 过滤 outbounds
	if err := cu.filterNodesFromSection(config, "outbounds", noNeedKeywords); err != nil {
		return fmt.Errorf("过滤outbounds失败: %v", err)
	}

	// 过滤 endpoints
	if err := cu.filterNodesFromSection(config, "endpoints", noNeedKeywords); err != nil {
		return fmt.Errorf("过滤endpoints失败: %v", err)
	}

	logger.Debug("no_need过滤完成")
	return nil
}

// filterNodesFromSection 从指定的配置段中过滤包含关键词的节点
func (cu *ConfigUpdater) filterNodesFromSection(config map[string]any, sectionName string, keywords []string) error {
	section, exists := config[sectionName]
	if !exists {
		logger.Debug("配置中不存在 %s 段，跳过过滤", sectionName)
		return nil
	}

	sectionSlice, ok := section.([]any)
	if !ok {
		logger.Debug("%s 段不是数组格式，跳过过滤", sectionName)
		return nil
	}

	var filteredNodes []any
	removedCount := 0

	for _, node := range sectionSlice {
		nodeMap, ok := node.(map[string]any)
		if !ok {
			// 保留非map格式的节点
			filteredNodes = append(filteredNodes, node)
			continue
		}

		// 检查节点的tag是否包含任何关键词
		shouldRemove := false
		if tag, exists := nodeMap["tag"]; exists {
			if tagStr, ok := tag.(string); ok {
				for _, keyword := range keywords {
					if keyword != "" && strings.Contains(tagStr, keyword) {
						logger.Debug("从%s中移除包含关键词'%s'的节点: %s", sectionName, keyword, tagStr)
						shouldRemove = true
						removedCount++
						break
					}
				}
			}
		}

		if !shouldRemove {
			filteredNodes = append(filteredNodes, node)
		}
	}

	if removedCount > 0 {
		config[sectionName] = filteredNodes
		logger.Debug("从%s中移除了 %d 个包含no_need关键词的节点", sectionName, removedCount)
	} else {
		logger.Debug("%s中没有找到包含no_need关键词的节点", sectionName)
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
