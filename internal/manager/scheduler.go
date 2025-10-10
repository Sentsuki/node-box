package manager

import (
	"context"
	"fmt"
	"time"

	"node-box/internal/config"
	"node-box/internal/logger"
)

// Scheduler provides periodic task scheduling for configuration updates.
// It manages the timing and execution of regular configuration update operations
// with proper context handling for graceful shutdown.
type Scheduler struct {
	manager    *NodeManager
	interval   time.Duration
	ctx        context.Context
	cancel     context.CancelFunc
	configPath string // 配置文件路径，用于重新加载
}

// NewScheduler creates a new scheduler instance with the specified manager and interval.
// Parameters:
//   - manager: NodeManager instance to execute update operations
//   - interval: time duration between update operations
//   - configPath: path to configuration file for reloading
func NewScheduler(manager *NodeManager, interval time.Duration, configPath string) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		manager:    manager,
		interval:   interval,
		ctx:        ctx,
		cancel:     cancel,
		configPath: configPath,
	}
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

// reloadConfigAndUpdate reloads the configuration file and updates all configurations.
// This ensures that each scheduled update uses the latest configuration.
func (s *Scheduler) reloadConfigAndUpdate() error {
	logger.Debug("重新加载配置文件: %s", s.configPath)

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

	// 创建新的节点管理器
	newManager, err := NewNodeManager(cfg)
	if err != nil {
		return fmt.Errorf("创建节点管理器失败: %v", err)
	}

	// 更新管理器引用
	s.manager = newManager

	logger.Debug("配置重新加载完成，开始执行更新...")

	// 执行配置更新
	return s.manager.UpdateAllConfigurations()
}
