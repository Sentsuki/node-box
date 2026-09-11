// Package fileops provides file operation functionality for configuration management.
// It handles scanning configuration directories and updating configuration files
// with new proxy node data.
package fileops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Scanner provides configuration file scanning functionality.
// It can recursively scan directories to find JSON configuration files,
// or handle single file paths directly.
type Scanner struct {
	configPath string
	isFile     bool
}

// NewScanner creates a new configuration file scanner for the specified path.
// The scanner will search for JSON files in the given directory and its subdirectories,
// or handle a single file if isFile is true.
func NewScanner(configPath string, isFile bool) *Scanner {
	return &Scanner{
		configPath: configPath,
		isFile:     isFile,
	}
}

// ScanConfigFiles scans for configuration files based on the scanner configuration.
// If isFile is true, it returns the single file path after validation.
// Otherwise, it scans the directory for all JSON configuration files.
func (s *Scanner) ScanConfigFiles() ([]string, error) {
	if s.isFile {
		// 单文件模式：验证文件存在性和格式
		if !strings.HasSuffix(strings.ToLower(s.configPath), ".json") {
			return nil, fmt.Errorf("file must have .json extension: %s", s.configPath)
		}

		if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s", s.configPath)
		}

		return []string{s.configPath}, nil
	}

	// 目录模式：递归扫描目录
	var configFiles []string

	err := filepath.Walk(s.configPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只处理JSON文件，跳过目录
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".json") {
			configFiles = append(configFiles, path)
		}

		return nil
	})

	return configFiles, err
}
