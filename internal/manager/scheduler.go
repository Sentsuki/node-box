package manager

import (
	"context"
	"log"
	"time"
)

// Scheduler provides periodic task scheduling for configuration updates.
// It manages the timing and execution of regular configuration update operations
// with proper context handling for graceful shutdown.
type Scheduler struct {
	manager  *NodeManager
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewScheduler creates a new scheduler instance with the specified manager and interval.
// Parameters:
//   - manager: NodeManager instance to execute update operations
//   - interval: time duration between update operations
func NewScheduler(manager *NodeManager, interval time.Duration) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		manager:  manager,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the periodic scheduling of configuration update tasks.
// It performs an initial update immediately, then continues with regular
// updates at the specified interval until Stop is called or context is cancelled.
func (s *Scheduler) Start() error {
	log.Printf("启动定时调度器，更新间隔: %v", s.interval)

	// 立即执行一次更新
	log.Println("执行初始配置更新...")
	if err := s.manager.UpdateAllConfigurations(); err != nil {
		log.Printf("初始配置更新失败: %v", err)
		// 不因为初始更新失败而停止调度器
	}

	// 创建定时器
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	log.Println("定时调度器已启动，等待下次更新...")

	for {
		select {
		case <-s.ctx.Done():
			log.Println("定时调度器已停止")
			return s.ctx.Err()

		case <-ticker.C:
			log.Println("开始定时配置更新...")

			if err := s.manager.UpdateAllConfigurations(); err != nil {
				log.Printf("定时配置更新失败: %v", err)
				// 继续运行，不因为单次更新失败而停止调度器
			} else {
				log.Println("定时配置更新完成")
			}
		}
	}
}

// Stop gracefully stops the scheduler by cancelling its context.
// This causes the Start method to exit and stops all scheduled operations.
func (s *Scheduler) Stop() {
	log.Println("正在停止定时调度器...")
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
