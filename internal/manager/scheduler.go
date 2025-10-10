package manager

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"time"

	"node-box/internal/config"
	"node-box/internal/logger"
)

// Scheduler provides periodic task scheduling for configuration updates.
// It manages the timing and execution of regular configuration update operations
// with proper context handling for graceful shutdown.
type Scheduler struct {
	manager        *NodeManager
	interval       time.Duration
	ctx            context.Context
	cancel         context.CancelFunc
	configPath     string    // 配置文件路径，用于重新加载
	lastConfigHash string    // 上次配置文件的哈希值，用于检测变化
	lastModTime    time.Time // 上次配置文件的修改时间
}

// NewScheduler creates a new scheduler instance with the specified manager and interval.
// Parameters:
//   - manager: NodeManager instance to execute update operations
//   - interval: time duration between update operations
//   - configPath: path to configuration file for reloading
func NewScheduler(manager *NodeManager, interval time.Duration, configPath string) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	scheduler := &Scheduler{
		manager:    manager,
		interval:   interval,
		ctx:        ctx,
		cancel:     cancel,
		configPath: configPath,
	}

	// 初始化配置文件状态
	scheduler.updateConfigFileState()

	return scheduler
}

// Start begins the periodic scheduling of configuration update tasks.
// It performs an initial update immediately, then continues with regular
// updates at the specified interval until Stop is called or context is cancelled.
func (s *Scheduler) Start() error {
	logger.Info("启动定时调度器，更新间隔: %v", s.interval)

	// 立即执行一次更新
	logger.Debug("执行初始配置更新...")
	if err := s.manager.UpdateAllConfigurations(); err != nil {
		logger.Error("初始配置更新失败: %v", err)
		// 不因为初始更新失败而停止调度器
	}

	// 创建定时器
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	logger.Info("定时调度器已启动，等待下次更新...")

	for {
		select {
		case <-s.ctx.Done():
			logger.Info("定时调度器已停止")
			return s.ctx.Err()

		case <-ticker.C:
			logger.Info("*****开始定时配置更新*****")

			// 重新加载配置文件
			if err := s.reloadConfigAndUpdate(); err != nil {
				logger.Error("定时配置更新失败: %v", err)
				// 继续运行，不因为单次更新失败而停止调度器
			} else {
				logger.Info("定时配置更新完成")
			}
		}
	}
}

// Stop gracefully stops the scheduler by cancelling its context.
// This causes the Start method to exit and stops all scheduled operations.
func (s *Scheduler) Stop() {
	logger.Info("正在停止定时调度器...")
	s.cancel()
}

// Cleanup performs complete cleanup of the scheduler and its resources.
// This method should be called when the scheduler is no longer needed.
func (s *Scheduler) Cleanup() {
	logger.Debug("开始清理调度器资源...")

	// 停止调度器
	s.Stop()

	// 清理节点管理器
	if s.manager != nil {
		s.manager.Cleanup()
		s.manager = nil
	}

	// 清理其他引用
	s.configPath = ""

	logger.Debug("调度器资源清理完成")
}

// IsRunning checks whether the scheduler is currently running.
// It returns true if the scheduler is active, false if it has been stopped.
func (s *Scheduler) IsRunning() bool {
	select {
	case <-s.ctx.Done():
		return false
	default:
		return true
	}
}

// updateConfigFileState 更新配置文件的状态信息（哈希值和修改时间）
func (s *Scheduler) updateConfigFileState() {
	hash, modTime, err := s.getConfigFileState()
	if err != nil {
		logger.Debug("获取配置文件状态失败: %v", err)
		return
	}
	s.lastConfigHash = hash
	s.lastModTime = modTime
}

// getConfigFileState 获取配置文件的哈希值和修改时间
func (s *Scheduler) getConfigFileState() (string, time.Time, error) {
	file, err := os.Open(s.configPath)
	if err != nil {
		return "", time.Time{}, err
	}
	defer file.Close()

	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return "", time.Time{}, err
	}

	// 计算文件哈希值
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", time.Time{}, err
	}

	hashString := fmt.Sprintf("%x", hash.Sum(nil))
	return hashString, fileInfo.ModTime(), nil
}

// hasConfigChanged 检查配置文件是否发生变化
func (s *Scheduler) hasConfigChanged() bool {
	currentHash, currentModTime, err := s.getConfigFileState()
	if err != nil {
		logger.Debug("检查配置文件变化时出错: %v", err)
		return true // 出错时假设配置已变化，确保更新
	}

	// 比较哈希值和修改时间
	changed := currentHash != s.lastConfigHash || !currentModTime.Equal(s.lastModTime)

	if changed {
		logger.Debug("检测到配置文件变化 (哈希: %s -> %s, 修改时间: %v -> %v)",
			s.lastConfigHash[:8], currentHash[:8], s.lastModTime, currentModTime)
	} else {
		logger.Debug("配置文件未发生变化，使用现有配置")
	}

	return changed
}

// reloadConfigAndUpdate 检查配置文件变化，只有在变化时才重新加载配置。
// 这样可以避免不必要的资源消耗。
func (s *Scheduler) reloadConfigAndUpdate() error {
	// 检查配置文件是否发生变化
	if !s.hasConfigChanged() {
		logger.Info("配置文件未变化，直接执行更新...")
		// 配置未变化，直接使用现有的 NodeManager 执行更新
		return s.manager.UpdateAllConfigurations()
	}

	logger.Info("检测到配置文件变化，重新加载配置: %s", s.configPath)

	// 重新加载配置
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return fmt.Errorf("重新加载配置失败: %v", err)
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("配置验证失败: %v", err)
	}

	// 检查更新间隔是否发生变化
	newInterval := time.Duration(cfg.UpdateInterval) * time.Hour
	if newInterval != s.interval {
		logger.Info("检测到更新间隔变化: %v -> %v", s.interval, newInterval)
		s.interval = newInterval
		logger.Info("更新间隔已更新，将在下次定时器重置时生效")
	}

	// 清理旧的节点管理器，避免内存泄漏
	if s.manager != nil {
		logger.Debug("清理旧的节点管理器...")
		s.manager.Cleanup()
	}

	// 创建新的节点管理器
	newManager, err := NewNodeManager(cfg)
	if err != nil {
		return fmt.Errorf("创建节点管理器失败: %v", err)
	}

	// 更新管理器引用
	s.manager = newManager

	// 更新配置文件状态
	s.updateConfigFileState()

	logger.Info("配置重新加载完成，开始执行更新...")

	// 执行配置更新
	return s.manager.UpdateAllConfigurations()
}
