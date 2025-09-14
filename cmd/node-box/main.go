package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"node-box/internal/config"
	"node-box/internal/manager"
)

const (
	defaultConfigPath = "config.json"
	appName           = "node-box"
	version           = "0.2.0"
)

// printUsage 显示程序使用帮助信息
func printUsage() {
	fmt.Printf(`%s v%s - SingBox节点订阅管理工具

用法:
  %s [配置文件路径]           使用指定配置文件运行程序
  %s init [配置文件路径]      生成示例配置文件
  %s -h, --help             显示此帮助信息
  %s -v, --version          显示版本信息

参数:
  配置文件路径                配置文件的路径，默认为 %s

示例:
  %s                        使用默认配置文件运行
  %s config.json            使用指定配置文件运行
  %s init                   生成默认配置文件
  %s init my-config.json    生成指定路径的配置文件

`, appName, version, appName, appName, appName, appName, defaultConfigPath, appName, appName, appName, appName)
}

// printVersion 显示版本信息
func printVersion() {
	fmt.Printf("%s v%s\n", appName, version)
}

// parseArgs 解析命令行参数，返回命令类型和配置文件路径
func parseArgs() (command string, configPath string) {
	configPath = defaultConfigPath

	if len(os.Args) < 2 {
		return "run", configPath
	}

	firstArg := os.Args[1]

	// 处理帮助参数
	if firstArg == "-h" || firstArg == "--help" {
		return "help", ""
	}

	// 处理版本参数
	if firstArg == "-v" || firstArg == "--version" {
		return "version", ""
	}

	// 处理init命令
	if firstArg == "init" {
		if len(os.Args) > 2 {
			configPath = os.Args[2]
		}
		return "init", configPath
	}

	// 其他情况视为配置文件路径
	return "run", firstArg
}

func main() {
	// 解析命令行参数
	command, configPath := parseArgs()

	switch command {
	case "help":
		printUsage()
		return

	case "version":
		printVersion()
		return

	case "init":
		log.Printf("正在生成示例配置文件: %s", configPath)
		if err := config.GenerateExample(configPath); err != nil {
			log.Fatalf("生成示例配置失败: %v", err)
		}
		log.Printf("示例配置文件已生成，请编辑 %s 后重新运行程序", configPath)
		return

	case "run":
		// 继续执行主程序逻辑
		log.Printf("使用配置文件: %s", configPath)

	default:
		log.Fatalf("未知命令: %s", command)
	}

	// 加载配置
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置验证失败: %v", err)
	}

	// 初始化节点管理器
	nodeManager, err := manager.NewNodeManager(cfg)
	if err != nil {
		log.Fatalf("初始化节点管理器失败: %v", err)
	}

	// 创建并启动调度器
	updateInterval := time.Duration(cfg.UpdateInterval) * time.Hour
	scheduler := manager.NewScheduler(nodeManager, updateInterval)

	log.Printf("程序启动成功，更新间隔: %v", updateInterval)
	log.Println("按 Ctrl+C 停止程序")

	// 启动调度器（这会阻塞直到程序结束）
	if err := scheduler.Start(); err != nil {
		log.Fatalf("调度器运行失败: %v", err)
	}
}
