// Package modules provides configuration file updating functionality for modules.
// It handles merging module configurations into target configuration files.
package modules

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	ErrUnsupportedFileType = errors.New("unsupported file type")
)

// ConfigUpdater handles updating configuration files with module data.
// It can merge module configurations into target configuration files
// and supports both JSON and YAML formats.
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
	log.Printf("开始更新配置文件: %s (%s)", configFile.Name, configFile.Path)

	// Check if config file exists
	if _, err := os.Stat(configFile.Path); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrConfigFileNotFound, configFile.Path)
	}

	// Read the target configuration file
	data, err := os.ReadFile(configFile.Path)
	if err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileRead, configFile.Path, err)
	}

	// Parse the configuration file
	var targetConfig map[string]any
	fileExt := strings.ToLower(filepath.Ext(configFile.Path))

	switch fileExt {
	case ".json":
		if err := json.Unmarshal(data, &targetConfig); err != nil {
			return fmt.Errorf("%w %s: %v", ErrConfigFileParse, configFile.Path, err)
		}
	case ".yaml", ".yml":
		// For YAML files, we'll need to implement YAML parsing
		// For now, we'll treat them as JSON and let the user know
		log.Printf("警告: YAML文件 %s 暂时按JSON格式处理", configFile.Path)
		if err := json.Unmarshal(data, &targetConfig); err != nil {
			return fmt.Errorf("%w %s: %v", ErrConfigFileParse, configFile.Path, err)
		}
	default:
		return fmt.Errorf("%w: %s (supported: .json, .yaml, .yml)", ErrUnsupportedFileType, fileExt)
	}

	// Apply modules to the configuration
	updatedConfig, err := cu.applyModules(targetConfig, configFile.Modules)
	if err != nil {
		return fmt.Errorf("应用模块失败 %s: %v", configFile.Name, err)
	}

	// Write the updated configuration back to file
	if err := cu.writeConfigFile(configFile.Path, updatedConfig, fileExt); err != nil {
		return fmt.Errorf("写入配置文件失败 %s: %v", configFile.Path, err)
	}

	log.Printf("成功更新配置文件: %s", configFile.Path)
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

// applyModuleByType applies a module based on its type (log, dns, etc.).
// It merges the module data into the appropriate section of the configuration.
func (cu *ConfigUpdater) applyModuleByType(targetConfig map[string]any, moduleData map[string]any, moduleName string) error {
	// Determine module type based on the module name or content
	moduleType := cu.detectModuleType(moduleData, moduleName)

	log.Printf("应用模块 %s (类型: %s)", moduleName, moduleType)

	switch moduleType {
	case "log":
		return cu.applyLogModule(targetConfig, moduleData)
	case "dns":
		return cu.applyDNSModule(targetConfig, moduleData)
	default:
		// For unknown types, try to merge the entire module data
		return cu.applyGenericModule(targetConfig, moduleData, moduleName)
	}
}

// detectModuleType detects the type of a module based on its content or name.
func (cu *ConfigUpdater) detectModuleType(moduleData map[string]any, moduleName string) string {
	// Check if the module has specific fields that indicate its type
	if _, hasLog := moduleData["log"]; hasLog {
		return "log"
	}
	if _, hasDNS := moduleData["dns"]; hasDNS {
		return "dns"
	}
	if _, hasOutbounds := moduleData["outbounds"]; hasOutbounds {
		return "outbounds"
	}

	// Check module name for type hints
	nameLower := strings.ToLower(moduleName)
	if strings.Contains(nameLower, "log") {
		return "log"
	}
	if strings.Contains(nameLower, "dns") {
		return "dns"
	}

	// Default to generic
	return "generic"
}

// applyLogModule applies a log module to the target configuration.
func (cu *ConfigUpdater) applyLogModule(targetConfig map[string]any, moduleData map[string]any) error {
	if logConfig, exists := moduleData["log"]; exists {
		targetConfig["log"] = logConfig
		log.Printf("已应用日志配置")
	} else {
		// If the module data itself is the log configuration
		targetConfig["log"] = moduleData
		log.Printf("已应用日志配置 (直接模式)")
	}
	return nil
}

// applyDNSModule applies a DNS module to the target configuration.
func (cu *ConfigUpdater) applyDNSModule(targetConfig map[string]any, moduleData map[string]any) error {
	if dnsConfig, exists := moduleData["dns"]; exists {
		targetConfig["dns"] = dnsConfig
		log.Printf("已应用DNS配置")
	} else {
		// If the module data itself is the DNS configuration
		targetConfig["dns"] = moduleData
		log.Printf("已应用DNS配置 (直接模式)")
	}
	return nil
}

// applyGenericModule applies a generic module to the target configuration.
func (cu *ConfigUpdater) applyGenericModule(targetConfig map[string]any, moduleData map[string]any, moduleName string) error {
	// For generic modules, merge the data at the top level
	for key, value := range moduleData {
		// Avoid overwriting critical configuration sections
		if key == "outbounds" || key == "routing" {
			log.Printf("跳过关键配置段: %s", key)
			continue
		}
		targetConfig[key] = value
	}
	log.Printf("已应用通用模块配置: %s", moduleName)
	return nil
}

// writeConfigFile writes the updated configuration to a file.
// It supports both JSON and YAML formats.
func (cu *ConfigUpdater) writeConfigFile(filePath string, config map[string]any, fileExt string) error {
	var data []byte
	var err error

	switch fileExt {
	case ".json":
		data, err = json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("%w: %v", ErrConfigFileWrite, err)
		}
	case ".yaml", ".yml":
		// For YAML files, we'll need to implement YAML marshaling
		// For now, we'll write as JSON with .yaml extension
		log.Printf("警告: YAML文件 %s 将以JSON格式写入", filePath)
		data, err = json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("%w: %v", ErrConfigFileWrite, err)
		}
	default:
		return fmt.Errorf("%w: unsupported file extension %s", ErrConfigFileWrite, fileExt)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileWrite, filePath, err)
	}

	return nil
}
