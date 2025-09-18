# API 文档

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