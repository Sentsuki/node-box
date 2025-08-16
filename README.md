# Node Box

一个用于管理和更新 SingBox 配置文件的工具，支持从 Clash 和 SingBox 订阅自动获取节点信息。

## 功能特性

- 支持 Clash 和 SingBox 订阅格式
- 自动转换 Clash 代理配置到 SingBox 格式
- 支持 HTTP/HTTPS/SOCKS5 代理获取订阅
- 自动更新配置文件中的节点列表
- 支持关键词过滤排除特定节点
- 定期自动更新功能

## 安装

确保已安装 Go 1.16 或更高版本：

```bash
go mod tidy
go build -o node-box main.go
```

## 配置

### 初始化配置文件

```bash
./node-box init config.json
```

### 配置文件格式

```json
{
  "subscriptions": [
    {
      "name": "订阅名称",
      "url": "订阅链接",
      "type": "clash",
      "enable": true
    }
  ],
  "config_dir": "./configs",
  "insert_marker": "🚀 节点选择",
  "update_interval_hours": 6,
  "exclude_keywords": ["故障转移", "流量"],
  "proxy": {
    "type": "http",
    "host": "127.0.0.1",
    "port": 7890,
    "username": "用户名",
    "password": "密码"
  }
}
```

### 代理配置说明

- `type`: 代理类型，支持 `http`、`https`、`socks5`
- `host`: 代理服务器地址
- `port`: 代理服务器端口
- `username`: 用户名（可选）
- `password`: 密码（可选）

如果不配置代理，程序将使用直连方式获取订阅。

## 使用方法

### 基本使用

```bash
# 使用默认配置文件 config.json
./node-box

# 使用自定义配置文件
./node-box custom-config.json
```

### 初始化配置

```bash
# 生成默认配置文件
./node-box init

# 生成到指定路径
./node-box init /path/to/config.json
```

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

## 注意事项

- 确保配置文件中的 `insert_marker` 是一个 selector 类型的节点
- 代理配置是可选的，如果不配置则使用直连
- 程序会自动过滤包含排除关键词的节点
- 建议设置合理的更新间隔，避免过于频繁的请求

## 许可证

GPL-3.0 license
