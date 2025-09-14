// Package fileops provides file operation functionality for configuration management.
// It handles scanning configuration directories and updating configuration files
// with new proxy node data.
package fileops

import (
	"os"
	"path/filepath"
	"strings"
)

// Scanner provides configuration file scanning functionality.
// It can recursively scan directories to find JSON configuration files.
type Scanner struct {
	configDir string
}

// NewScanner creates a new configuration file scanner for the specified directory.
// The scanner will search for JSON files in the given directory and its subdirectories.
func NewScanner(configDir string) *Scanner {
	return &Scanner{
		configDir: configDir,
	}
}

// ScanConfigFiles scans the configuration directory for all JSON configuration files.
// It returns a list of file paths for all JSON files found in the directory tree.
func (s *Scanner) ScanConfigFiles() ([]string, error) {
	var configFiles []string

	err := filepath.Walk(s.configDir, func(path string, info os.FileInfo, err error) error {
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
