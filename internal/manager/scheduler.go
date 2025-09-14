package manager

import (
	"context"
	"log"
	"time"
)

// Scheduler 定时调度器，负责定期执行配置更新任务
type Scheduler struct {
	manager  *NodeManager
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewScheduler 创建新的调度器实例
// manager: 节点管理器实例
// interval: 更新间隔时间
func NewScheduler(manager *NodeManager, interval time.Duration) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		manager:  manager,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start 启动定时调度器
// 开始定期执行配置更新任务，直到调用Stop或上下文被取消
func (s *Scheduler) Start() error {
	log.Printf("启动定时调度器，更新间隔: %v", s.interval)

	// 立即执行一次更新
	log.Println("执行初始配置更新...")
	if err := s.manager.UpdateAllConfigs(); err != nil {
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

			if err := s.manager.UpdateAllConfigs(); err != nil {
				log.Printf("定时配置更新失败: %v", err)
				// 继续运行，不因为单次更新失败而停止调度器
			} else {
				log.Println("定时配置更新完成")
			}
		}
	}
}

// Stop 停止定时调度器
// 取消上下文，使Start方法退出
func (s *Scheduler) Stop() {
	log.Println("正在停止定时调度器...")
	s.cancel()
}

// IsRunning 检查调度器是否正在运行
func (s *Scheduler) IsRunning() bool {
	select {
	case <-s.ctx.Done():
		return false
	default:
		return true
	}
}
