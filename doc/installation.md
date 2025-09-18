# 安装指南

## 前置要求

- Go 1.21 或更高版本

## 构建

### 基本构建

```bash
# 克隆项目
git clone <repository-url>
cd node-box

# 安装依赖
go mod tidy

# 构建可执行文件
go build -o bin/node-box ./cmd/node-box

# 或者直接运行
go run ./cmd/node-box
```

### 交叉编译

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o bin/node-box-linux ./cmd/node-box

# Windows
GOOS=windows GOARCH=amd64 go build -o bin/node-box.exe ./cmd/node-box

# macOS
GOOS=darwin GOARCH=amd64 go build -o bin/node-box-darwin ./cmd/node-box
```

## 项目结构

本项目遵循 Go 标准项目布局，采用模块化设计：

```
node-box/
├── cmd/
│   └── node-box/           # 程序入口点
│       └── main.go
├── internal/               # 内部包，不对外暴露
│   ├── config/            # 配置管理
│   │   ├── config.go      # 配置结构体和加载逻辑
│   │   └── example.go     # 示例配置生成
│   ├── client/            # HTTP 客户端和网络请求
│   │   ├── http.go        # HTTP 客户端实现
│   │   └── fetcher.go     # 订阅获取器
│   ├── subscription/      # 订阅数据处理
│   │   ├── types.go       # 类型定义
│   │   ├── processor.go   # 处理器接口
│   │   ├── clash.go       # Clash 订阅处理
│   │   ├── singbox.go     # SingBox 订阅处理
│   │   └── filter.go      # 节点过滤逻辑
│   ├── fileops/           # 文件操作
│   │   ├── scanner.go     # 配置文件扫描
│   │   └── updater.go     # 配置文件更新
│   └── manager/           # 核心业务逻辑
│       ├── manager.go     # 节点管理器
│       └── scheduler.go   # 定时任务调度
├── go.mod
├── go.sum
└── README.md
```