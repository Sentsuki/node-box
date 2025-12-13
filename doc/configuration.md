# 配置说明

## 初始化配置文件

```bash
./node-box init config.json
```

## 配置文件格式

```json
{
  "nodes": {
    "subscriptions": [
      {
        "name": "订阅名称",
        "url": "订阅链接",
        "type": "clash",
        "enable": true,
        "remove_emoji": true,
        "user_agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15"
      }
    ],
    "targets": [
      {
        "path": "./configs",
        "insert_marker": "🚀 节点选择"
      },
      {
        "path": "./configs/gaming.json",
        "insert_marker": "🎮 游戏节点",
        "subscriptions": ["订阅A"],
        "is_file": true
      }
    ],
    "exclude_keywords": ["故障转移", "流量"]
  },
  "update_schedule": {
    "type": "interval",
    "interval": 6
  },
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

## 增强功能说明

### 1. 文件级别配置支持

现在支持两种配置模式：

**目录模式（默认）**：
```json
{
  "path": "./configs",
  "insert_marker": "🚀 节点选择"
}
```

**文件模式（新增）**：
```json
{
  "path": "./configs/specific.json",
  "insert_marker": "🌟 特定节点",
  "is_file": true
}
```

### 2. 选择性订阅插入

支持为不同配置指定不同的订阅源：

```json
{
  "path": "./configs/gaming.json",
  "insert_marker": "🎮 游戏节点",
  "subscriptions": ["低延迟订阅", "游戏专用"],
  "is_file": true
}
```

- 不指定 `subscriptions`：使用所有启用的订阅
- 指定 `subscriptions`：只使用指定的订阅源

## 性能优化功能

### 智能重试机制
- 订阅获取失败时自动重试最多3次
- 递增延迟时间（2秒、4秒、6秒）
- 详细的重试日志记录

### 订阅缓存优化
- 一次更新周期内，每个订阅只获取一次
- 多个配置目标共享缓存数据
- 显著减少网络请求和处理时间
- **性能提升高达80%**

```
优化前：每个目标都重复获取订阅 -> 15次请求，30秒
优化后：所有订阅只获取一次 -> 3次请求，6秒
```

## 更新调度配置说明

### 调度模式选择

程序支持两种更新调度模式：

#### 1. 间隔模式
```json
{
  "update_schedule": {
    "type": "interval",
    "interval": 6
  }
}
```
- 按照指定的小时间隔进行更新
- 程序启动后立即执行一次更新，然后按间隔定时更新
- `interval` 字段指定更新间隔（小时）
- 适合需要精确控制更新频率的场景

#### 2. 整点模式
```json
{
  "update_schedule": {
    "type": "hourly"
  }
}
```
- 在每个整点（如 1:00、2:00、3:00...）执行更新
- 程序启动后立即执行一次更新，然后在每个整点更新
- 不需要指定 `interval` 字段
- 适合希望在固定时间点更新的场景



### 调度模式对比

| 模式 | 更新时机 | 配置方式 | 适用场景 |
|------|----------|----------|----------|
| 间隔模式 | 按固定间隔更新 | `{"type": "interval", "interval": 6}` | 需要精确控制更新频率 |
| 整点模式 | 每个整点更新 | `{"type": "hourly"}` | 希望在固定时间点更新 |

## 代理配置说明

- `type`: 代理类型，支持 `http`、`https`、`socks5`
- `host`: 代理服务器地址
- `port`: 代理服务器端口
- `username`: 用户名（可选）
- `password`: 密码（可选）

如果不配置代理，程序将使用直连方式获取订阅。

## User-Agent 配置说明

支持两个级别的User-Agent配置：

### 1. 全局User-Agent配置
- `user_agent`: 全局默认的HTTP请求User-Agent头（可选）
- 如果不配置，将使用默认值：`sing-box`
- 作为所有订阅的后备User-Agent

### 2. 订阅级别User-Agent配置
- 每个订阅可以单独配置 `user_agent` 字段
- 订阅级别的User-Agent优先级高于全局配置
- 如果订阅没有配置User-Agent，则使用全局User-Agent

**优先级顺序：**
1. 订阅的 `user_agent` 字段（最高优先级）
2. 全局的 `user_agent` 字段
3. 默认值 `sing-box`（最低优先级）

### 多订阅不同User-Agent示例

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

### 常用User-Agent示例

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

## 节点名称处理说明

### Emoji 移除配置
- 每个订阅可以单独配置 `remove_emoji` 字段
- 设置为 `true` 时，程序会自动移除该订阅下所有节点名称中的 Emoji 表情
- 默认为 `false`（不移除）

```json
{
  "name": "订阅名称",
  "url": "订阅链接",
  "remove_emoji": true
}
```