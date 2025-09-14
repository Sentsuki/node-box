// Package fileops 提供文件操作相关功能
package fileops

import (
	"os"
	"path/filepath"
	"strings"
)

// Scanner 配置文件扫描器
type Scanner struct {
	configDir string
}

// NewScanner 创建新的配置文件扫描器
func NewScanner(configDir string) *Scanner {
	return &Scanner{
		configDir: configDir,
	}
}

// ScanConfigFiles 扫描配置目录中的所有JSON配置文件
// 返回找到的配置文件路径列表
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
