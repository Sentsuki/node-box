# 设计文档

## 概述

本设计文档描述了如何将现有的node-box项目从单文件结构重构为遵循Go标准项目布局的多包架构。重构将提高代码的可维护性、可测试性和可扩展性，同时保持所有现有功能不变。

## 架构

### 项目结构

```
node-box/
├── cmd/
│   └── node-box/
│       └── main.go              # 程序入口点
├── internal/
│   ├── config/
│   │   ├── config.go            # 配置结构体和方法
│   │   └── example.go           # 示例配置生成
│   ├── client/
│   │   ├── http.go              # HTTP客户端和代理配置
│   │   └── fetcher.go           # 订阅获取逻辑
│   ├── subscription/
│   │   ├── types.go             # 订阅相关类型定义
│   │   ├── clash.go             # Clash订阅处理
│   │   ├── singbox.go           # SingBox订阅处理
│   │   └── filter.go            # 节点过滤逻辑
│   ├── fileops/
│   │   ├── scanner.go           # 文件扫描
│   │   └── updater.go           # 配置文件更新
│   └── manager/
│       ├── manager.go           # 核心节点管理器
│       └── scheduler.go         # 定时任务调度器
├── go.mod
├── go.sum
├── config.json
├── README.md
└── LICENSE
```

### 设计原则

1. **关注点分离**: 每个包负责特定的功能领域
2. **依赖注入**: 通过接口实现松耦合
3. **错误处理**: 统一的错误处理策略
4. **可测试性**: 每个包都可以独立测试

## 组件和接口

### Config包

**职责**: 管理应用程序配置

```go
package config

// Config 应用程序配置
type Config struct {
    Subscriptions   []Subscription `json:"subscriptions"`
    ConfigDir       string         `json:"config_dir"`
    InsertMarker    string         `json:"insert_marker"`
    UpdateInterval  int            `json:"update_interval_hours"`
    ExcludeKeywords []string       `json:"exclude_keywords,omitempty"`
    Proxy           *ProxyConfig   `json:"proxy,omitempty"`
}

// 主要方法
func Load(path string) (*Config, error)
func (c *Config) Validate() error
func GenerateExample(path string) error
```

### Client包

**职责**: 处理HTTP请求和代理配置

```go
package client

// HTTPClient 接口定义
type HTTPClient interface {
    Get(url string) ([]byte, error)
}

// Fetcher 订阅获取器
type Fetcher struct {
    client HTTPClient
}

// 主要方法
func NewHTTPClient(proxy *config.ProxyConfig) (HTTPClient, error)
func NewFetcher(client HTTPClient) *Fetcher
func (f *Fetcher) FetchSubscription(url string) ([]byte, error)
```

### Subscription包

**职责**: 处理不同类型的订阅数据

```go
package subscription

// Processor 订阅处理器接口
type Processor interface {
    Process(data []byte) ([]Node, error)
}

// Node 统一的节点表示
type Node map[string]any

// 主要组件
type ClashProcessor struct{}
type SingBoxProcessor struct{}
type Filter struct{}

// 主要方法
func NewProcessor(subType string) (Processor, error)
func (f *Filter) FilterNodes(nodes []Node, keywords []string) []Node
func AddSubscriptionPrefix(nodes []Node, prefix string) []Node
```

### FileOps包

**职责**: 文件系统操作

```go
package fileops

// Scanner 配置文件扫描器
type Scanner struct {
    configDir string
}

// Updater 配置文件更新器
type Updater struct {
    insertMarker string
}

// 主要方法
func NewScanner(configDir string) *Scanner
func (s *Scanner) ScanConfigFiles() ([]string, error)
func NewUpdater(insertMarker string) *Updater
func (u *Updater) UpdateConfigFile(path string, nodes []Node) error
```

### Manager包

**职责**: 协调各个组件，实现核心业务逻辑

```go
package manager

// NodeManager 节点管理器
type NodeManager struct {
    config      *config.Config
    fetcher     *client.Fetcher
    processors  map[string]subscription.Processor
    scanner     *fileops.Scanner
    updater     *fileops.Updater
    filter      *subscription.Filter
}

// Scheduler 定时调度器
type Scheduler struct {
    manager  *NodeManager
    interval time.Duration
}

// 主要方法
func NewNodeManager(cfg *config.Config) (*NodeManager, error)
func (nm *NodeManager) UpdateAllConfigs() error
func (nm *NodeManager) FetchAllNodes() ([]subscription.Node, error)
func NewScheduler(manager *NodeManager, interval time.Duration) *Scheduler
func (s *Scheduler) Start() error
```

## 数据模型

### 配置模型

```go
// Config 主配置结构
type Config struct {
    Subscriptions   []Subscription `json:"subscriptions"`
    ConfigDir       string         `json:"config_dir"`
    InsertMarker    string         `json:"insert_marker"`
    UpdateInterval  int            `json:"update_interval_hours"`
    ExcludeKeywords []string       `json:"exclude_keywords,omitempty"`
    Proxy           *ProxyConfig   `json:"proxy,omitempty"`
}

// Subscription 订阅配置
type Subscription struct {
    Name   string `json:"name"`
    URL    string `json:"url"`
    Type   string `json:"type"`
    Enable bool   `json:"enable"`
}

// ProxyConfig 代理配置
type ProxyConfig struct {
    Type     string `json:"type"`
    Host     string `json:"host"`
    Port     int    `json:"port"`
    Username string `json:"username"`
    Password string `json:"password"`
}
```

### 节点模型

```go
// Node 统一的节点表示，使用any替代interface{}
type Node map[string]any

// ClashProxy Clash代理结构（用于解析）
type ClashProxy struct {
    Name           string            `yaml:"name"`
    Type           string            `yaml:"type"`
    Server         string            `yaml:"server"`
    Port           string            `yaml:"port"`
    // ... 其他字段
}
```

## 错误处理

### 错误类型定义

```go
// 在各个包中定义特定的错误类型
var (
    ErrConfigNotFound     = errors.New("config file not found")
    ErrInvalidConfigFormat = errors.New("invalid config format")
    ErrUnsupportedSubType = errors.New("unsupported subscription type")
    ErrProxyConfigInvalid = errors.New("invalid proxy configuration")
)
```

### 错误处理策略

1. **配置错误**: 程序启动时验证配置，发现错误立即退出
2. **网络错误**: 记录日志，继续处理其他订阅
3. **文件操作错误**: 记录日志，跳过有问题的文件
4. **数据解析错误**: 记录详细错误信息，继续处理

## 测试策略

### 单元测试

每个包都应该有对应的测试文件：

```
internal/
├── config/
│   ├── config.go
│   └── config_test.go
├── client/
│   ├── http.go
│   ├── http_test.go
│   ├── fetcher.go
│   └── fetcher_test.go
└── ...
```

### 测试覆盖范围

1. **Config包**: 配置加载、验证、示例生成
2. **Client包**: HTTP客户端创建、代理配置、请求处理
3. **Subscription包**: 各种订阅格式解析、节点转换、过滤
4. **FileOps包**: 文件扫描、配置更新
5. **Manager包**: 业务逻辑协调、错误处理

### Mock和接口

使用接口实现依赖注入，便于测试：

```go
// 可以mock的接口
type HTTPClient interface {
    Get(url string) ([]byte, error)
}

type FileSystem interface {
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, data []byte) error
    Walk(root string, fn filepath.WalkFunc) error
}
```

## 迁移策略

### 重构步骤

1. **创建包结构**: 建立目录和基础文件
2. **提取配置逻辑**: 将配置相关代码移到config包
3. **提取网络逻辑**: 将HTTP客户端代码移到client包
4. **提取订阅处理**: 将订阅解析代码移到subscription包
5. **提取文件操作**: 将文件操作代码移到fileops包
6. **重构核心逻辑**: 将NodeManager移到manager包
7. **简化main函数**: 只保留程序入口逻辑
8. **更新类型定义**: 将interface{}替换为any

### 兼容性保证

1. **API兼容**: 保持命令行参数和配置文件格式不变
2. **功能兼容**: 确保所有现有功能正常工作
3. **性能兼容**: 重构不应显著影响性能

## 代码质量改进

### Go语言最佳实践

1. **使用any替代interface{}**: 提高代码可读性
2. **包名简洁**: 使用简短、描述性的包名
3. **导出函数文档**: 为所有公共API添加注释
4. **错误处理**: 遵循Go语言错误处理惯例
5. **代码格式**: 使用gofmt和golint确保代码风格一致

### 性能优化

1. **减少内存分配**: 重用对象，避免不必要的内存分配
2. **并发处理**: 在适当的地方使用goroutine提高性能
3. **缓存机制**: 对频繁访问的数据进行缓存

## 部署和维护

### 构建

```bash
# 构建可执行文件
go build -o bin/node-box ./cmd/node-box

# 交叉编译
GOOS=linux GOARCH=amd64 go build -o bin/node-box-linux ./cmd/node-box
```

### 依赖管理

继续使用go.mod管理依赖，确保版本兼容性。

### 文档

1. **README更新**: 更新项目说明和使用方法
2. **API文档**: 为公共接口生成文档
3. **架构文档**: 维护架构设计文档