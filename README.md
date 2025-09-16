# Node Box

一个用于管理和更新 SingBox 配置文件的工具，支持从 Clash 和 SingBox 订阅自动获取节点信息。

## 功能特性

- 支持 Clash 和 SingBox 订阅格式
- 自动转换 Clash 代理配置到 SingBox 格式
- 支持 HTTP/HTTPS/SOCKS5 代理获取订阅
- **🆕 支持自定义 User-Agent（全局和订阅级别，避免订阅服务器拦截）**
- 自动更新配置文件中的节点列表
- 支持关键词过滤排除特定节点
- **🆕 支持文件级别的精确配置更新**
- **🆕 支持选择性订阅节点插入**
- **🚀 智能重试机制（失败时自动重试3次）**
- **🚀 订阅缓存优化（避免重复获取，提升80%性能）**
- 定期自动更新功能
- 模块化架构，易于维护和扩展

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

## 安装

### 前置要求

- Go 1.21 或更高版本

### 构建

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

## 配置

### 初始化配置文件

```bash
./node-box init config.json
```

### 配置文件格式

```json
{
  "nodes": {
    "subscriptions": [
      {
        "name": "订阅名称",
        "url": "订阅链接",
        "type": "clash",
        "enable": true,
        "user_agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15"
      }
    ],
    "targets": [
      {
        "insert_path": "./configs",
        "insert_marker": "🚀 节点选择"
      },
      {
        "insert_path": "./configs/gaming.json",
        "insert_marker": "🎮 游戏节点",
        "subscriptions": ["订阅A"],
        "is_file": true
      }
    ],
    "exclude_keywords": ["故障转移", "流量"]
  },
  "update_interval_hours": 6,
  "user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
  "proxy": {
    "type": "http",
    "host": "127.0.0.1",
    "port": 7890,
    "username": "用户名",
    "password": "密码"
  }
}
```

### 🆕 增强功能说明

#### 1. 文件级别配置支持

现在支持两种配置模式：

**目录模式（默认）**：
```json
{
  "insert_path": "./configs",
  "insert_marker": "🚀 节点选择"
}
```

**文件模式（新增）**：
```json
{
  "insert_path": "./configs/specific.json",
  "insert_marker": "🌟 特定节点",
  "is_file": true
}
```

#### 2. 选择性订阅插入

支持为不同配置指定不同的订阅源：

```json
{
  "insert_path": "./configs/gaming.json",
  "insert_marker": "🎮 游戏节点",
  "subscriptions": ["低延迟订阅", "游戏专用"],
  "is_file": true
}
```

- 不指定 `subscriptions`：使用所有启用的订阅
- 指定 `subscriptions`：只使用指定的订阅源

### 🚀 性能优化功能

#### 智能重试机制
- 订阅获取失败时自动重试最多3次
- 递增延迟时间（2秒、4秒、6秒）
- 详细的重试日志记录

#### 订阅缓存优化
- 一次更新周期内，每个订阅只获取一次
- 多个配置目标共享缓存数据
- 显著减少网络请求和处理时间
- **性能提升高达80%**

```
优化前：每个目标都重复获取订阅 -> 15次请求，30秒
优化后：所有订阅只获取一次 -> 3次请求，6秒
```

### 代理配置说明

- `type`: 代理类型，支持 `http`、`https`、`socks5`
- `host`: 代理服务器地址
- `port`: 代理服务器端口
- `username`: 用户名（可选）
- `password`: 密码（可选）

如果不配置代理，程序将使用直连方式获取订阅。

### User-Agent 配置说明

支持两个级别的User-Agent配置：

#### 1. 全局User-Agent配置
- `user_agent`: 全局默认的HTTP请求User-Agent头（可选）
- 如果不配置，将使用默认值：`sing-box`
- 作为所有订阅的后备User-Agent

#### 2. 订阅级别User-Agent配置（🆕）
- 每个订阅可以单独配置 `user_agent` 字段
- 订阅级别的User-Agent优先级高于全局配置
- 如果订阅没有配置User-Agent，则使用全局User-Agent

**优先级顺序：**
1. 订阅的 `user_agent` 字段（最高优先级）
2. 全局的 `user_agent` 字段
3. 默认值 `sing-box`（最低优先级）

**多订阅不同User-Agent示例：**
```json
{
  "nodes": {
    "subscriptions": [
      {
        "name": "桌面端订阅",
        "url": "https://example.com/desktop",
        "type": "clash",
        "enable": true,
        "user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
      },
      {
        "name": "移动端订阅",
        "url": "https://example.com/mobile",
        "type": "clash",
        "enable": true,
        "user_agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15"
      },
      {
        "name": "默认订阅",
        "url": "https://example.com/default",
        "type": "clash",
        "enable": true
      }
    ]
  },
  "user_agent": "sing-box (Global Default)"
}
```

**常用User-Agent示例：**

桌面端Chrome：
```json
"user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
```

移动端Safari：
```json
"user_agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
```

Android Chrome：
```json
"user_agent": "Mozilla/5.0 (Linux; Android 13; SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36"
```

## 使用方法

### 基本使用

```bash
# 使用默认配置文件 config.json
./bin/node-box

# 使用自定义配置文件
./bin/node-box custom-config.json
```

### 初始化配置

```bash
# 生成默认配置文件
./bin/node-box init

# 生成到指定路径
./bin/node-box init /path/to/config.json
```

## 包级别说明

### config 包

负责配置文件的加载、验证和示例生成：

- `Load(path string)`: 加载配置文件
- `Validate()`: 验证配置有效性
- `GenerateExample(path string)`: 生成示例配置文件

### client 包

处理 HTTP 请求和代理配置：

- `NewHTTPClient(proxy *config.ProxyConfig, userAgent string)`: 创建 HTTP 客户端
- `NewFetcher(client HTTPClient)`: 创建订阅获取器
- `FetchSubscription(url string)`: 获取订阅内容

### subscription 包

处理不同类型的订阅数据：

- `NewProcessor(subType string)`: 创建订阅处理器
- `Process(data []byte)`: 处理订阅数据
- `FilterNodes(nodes []Node, keywords []string)`: 过滤节点
- `AddSubscriptionPrefix(nodes []Node, prefix string)`: 添加订阅前缀

### fileops 包

文件系统操作：

- `NewScanner(configDir string)`: 创建文件扫描器
- `ScanConfigFiles()`: 扫描配置文件
- `NewUpdater(insertMarker string)`: 创建配置更新器
- `UpdateConfigFile(path string, nodes []Node)`: 更新配置文件

### manager 包

核心业务逻辑协调：

- `NewNodeManager(cfg *config.Config)`: 创建节点管理器
- `UpdateAllConfigs()`: 更新所有配置
- `FetchAllNodes()`: 获取所有节点
- `NewScheduler(manager *NodeManager, interval time.Duration)`: 创建调度器

## 工作原理

1. 程序启动时读取配置文件
2. 根据配置的代理设置创建 HTTP 客户端
3. 通过代理或直连获取订阅内容
4. 解析订阅数据并转换为 SingBox 格式
5. 更新配置文件中的节点列表
6. 定期执行更新任务

## 支持的订阅类型

### Clash
- Shadowsocks
- VMess
- VLESS
- Trojan
- 支持 WebSocket 和 TLS 配置

### SingBox
- 保留所有原始字段
- 自动添加订阅前缀到节点标签

## 开发

### 代码结构

项目采用分层架构设计：

1. **cmd/node-box**: 程序入口，处理命令行参数
2. **internal/config**: 配置管理层
3. **internal/client**: 网络请求层
4. **internal/subscription**: 数据处理层
5. **internal/fileops**: 文件操作层
6. **internal/manager**: 业务逻辑层

### 添加新的订阅类型

1. 在 `internal/subscription` 包中实现 `Processor` 接口
2. 在 `NewProcessor` 函数中注册新的处理器
3. 添加相应的测试用例

### 测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./internal/config

# 运行测试并显示覆盖率
go test -cover ./...
```

### 代码质量检查

```bash
# 格式化代码
go fmt ./...

# 静态分析
go vet ./...

# 使用 golint（需要安装）
golint ./...
```

## 注意事项

- 确保配置文件中的 `insert_marker` 是一个 selector 类型的节点
- 代理配置是可选的，如果不配置则使用直连
- 程序会自动过滤包含排除关键词的节点
- 建议设置合理的更新间隔，避免过于频繁的请求
- 所有内部包都位于 `internal/` 目录下，不对外暴露

## 贡献

1. Fork 本项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

## 许可证

GPL-3.0 license
