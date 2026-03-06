# 配置文件参考手册

node-box 使用 JSON 格式的配置文件。可通过命令 `node-box init config.json` 生成示例配置。

---

## 顶层结构

```json
{
  "nodes":           { ... },   // 必填 - 节点订阅与目标配置
  "modules":         { ... },   // 可选 - sing-box 模块定义
  "configs":         [ ... ],   // 可选 - 配置文件合成规则
  "update_schedule": { ... },   // 必填 - 更新调度
  "log_level":       "info",    // 可选 - 日志级别
  "proxy":           { ... },   // 可选 - 代理设置
  "user_agent":      "..."      // 可选 - 全局 User-Agent
}
```

---

## `nodes` — 节点配置

节点配置是 node-box 的核心，负责定义订阅源、目标配置文件和过滤规则。

### `nodes.subscriptions` — 订阅源列表

定义所有订阅来源，每个订阅为一个对象。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | ✅ | 订阅唯一名称，用于在 targets 中引用和在节点 tag 中作为前缀标识 |
| `url` | string | ⚠️ | 远程订阅 URL，与 `path` 二选一 |
| `path` | string | ⚠️ | 本地订阅文件路径，与 `url` 二选一 |
| `type` | string | ✅ | 订阅格式：`"clash"` / `"singbox"` / `"relay"` |
| `enable` | bool | ✅ | 是否启用该订阅 |
| `remove_emoji` | bool | ❌ | 是否移除节点名称中的 Emoji 表情，默认 `false` |
| `user_agent` | string | ❌ | 自定义 User-Agent，优先级高于全局设置 |

**订阅类型说明：**

- **`clash`** — Clash YAML 格式的订阅，会自动转换为 sing-box 格式。支持的协议：vmess、vless、shadowsocks、trojan、http、socks5、hysteria、hysteria2、wireguard、tuic、anytls
- **`singbox`** — 原生 sing-box JSON 格式的订阅，直接提取 `outbounds` 中的代理节点（排除 `direct`、`block`、`selector` 类型）
- **`relay`** — 使用与 `singbox` 相同的处理器，但节点不会作为普通节点写入，而是作为中继模板，与其他普通节点做笛卡尔积展开（详见 [Relay 处理](#relay-处理机制)）

**示例：**

```json
{
  "subscriptions": [
    {
      "name": "主力订阅",
      "url": "https://example.com/clash/sub",
      "type": "clash",
      "enable": true,
      "remove_emoji": true,
      "user_agent": "ClashForWindows/0.20"
    },
    {
      "name": "入口订阅",
      "url": "https://example.com/relay/sub",
      "type": "relay",
      "enable": true
    },
    {
      "name": "本地备用",
      "path": "./subscriptions/backup.json",
      "type": "singbox",
      "enable": false
    }
  ]
}
```

---

### `nodes.targets` — 目标配置

定义节点要写入的目标 sing-box 配置文件或目录。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | ❌ | 目标名称（仅标识用途） |
| `path` | string | ✅ | 目标路径，可以是目录或单个文件 |
| `is_file` | bool | ❌ | 设为 `true` 表示 `path` 指向单个文件，默认 `false`（目录模式） |
| `subscriptions` | string[] | ❌ | 指定使用的订阅名称列表。为空则使用所有已启用订阅 |
| `proxies` | ProxyTarget[] | ✅ | 代理插入规则列表，至少一条 |

**路径模式说明：**

- **目录模式**（默认）：递归扫描路径下的所有 `.json` 文件
- **文件模式**（`is_file: true`）：直接操作指定的单个 `.json` 文件

---

### `nodes.targets[].proxies` — 代理插入规则

每个 target 可以定义多个 proxy 规则，每个规则对应 sing-box 配置中的一个 selector outbound。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `insert_marker` | string | ✅ | 目标 selector 的 tag 名称。程序会在 outbounds 中找到此 tag 对应的 selector，并将匹配节点的 tag 插入到其 outbounds 列表中 |
| `include_keywords` | string[] | ❌ | 包含关键词过滤。**只有** tag 包含列表中任一关键词的节点会被加入此 selector |
| `exclude_keywords` | string[] | ❌ | 排除关键词过滤。tag 包含列表中任一关键词的节点会被从此 selector 中排除 |
| `relay_nodes` | string[] | ❌ | Relay 节点过滤关键词。决定哪些 relay 展开后的节点 tag 会出现在此 selector 中（详见 [Relay 处理](#relay-处理机制)） |

**过滤优先级：** `include_keywords` 和 `exclude_keywords` 仅影响 selector 的 outbounds tag 列表，不影响真实节点的写入。真实代理节点始终会完整写入配置文件的 outbounds 数组。

**示例：**

```json
{
  "targets": [
    {
      "name": "主配置",
      "path": "./configs",
      "subscriptions": ["主力订阅", "入口订阅"],
      "proxies": [
        {
          "insert_marker": "🚀 节点选择",
          "include_keywords": ["香港", "新加坡", "美国"],
          "exclude_keywords": ["过期", "测试"],
          "relay_nodes": ["HK入口", "SG入口"]
        },
        {
          "insert_marker": "🎬 流媒体",
          "include_keywords": ["Netflix", "Disney"]
        }
      ]
    },
    {
      "name": "游戏配置",
      "path": "./configs/gaming.json",
      "is_file": true,
      "subscriptions": ["低延迟订阅"],
      "proxies": [
        {
          "insert_marker": "🎮 游戏节点"
        }
      ]
    }
  ]
}
```

---

### `nodes.exclude_keywords` — 全局排除关键词

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `exclude_keywords` | string[] | ❌ | 全局排除关键词列表。tag 中包含任一关键词的节点会被排除，不写入任何配置文件 |

与 `proxies[].exclude_keywords` 的区别：全局排除会在真实节点写入前执行，被排除的节点不会出现在配置文件的 outbounds 中；而 proxy 级别的排除只影响特定 selector 的 outbounds tag 列表。

```json
{
  "exclude_keywords": ["过期", "失效", "测试", "故障转移", "流量"]
}
```

---

### `nodes.include_relay` — Relay 节点写入规则

决定哪些 relay 展开后的节点会作为真实 outbound 写入 sing-box 配置文件。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `tag` | string | ✅ | relay 节点的 tag 中必须包含此关键词 |
| `upstream` | string[] | ✅ | relay 节点的 tag 中必须同时包含此列表中的至少一个关键词 |

一个节点要被写入，必须同时满足：tag 包含 `tag` 关键词 **并且** tag 包含 `upstream` 中的任一关键词。

如果 `include_relay` 为空数组或未配置，则不会写入任何 relay 节点。

```json
{
  "include_relay": [
    {
      "tag": "HK入口",
      "upstream": ["US-1", "SG-1", "JP-1"]
    },
    {
      "tag": "备用入口",
      "upstream": ["backup"]
    }
  ]
}
```

---

## Relay 处理机制

Relay 是 node-box 的高级功能，用于构建链式代理（多跳转发）。整体流程分四步：

### 1. 模板展开

`type: "relay"` 的订阅节点被视为中继模板。程序将每个模板节点与所有普通节点做笛卡尔积组合，生成展开节点：

- 展开后节点的 `detour` 字段 = 普通节点的 tag（作为上游出口）
- 展开后节点的 `tag` = `[relay订阅名] 模板原始名称 普通节点tag`

例如：relay 模板 `HK入口` × 普通节点 `[落地] US-1` → 展开为 tag `[入口] HK入口 [落地] US-1`，detour 为 `[落地] US-1`

### 2. `relay_nodes` 全局筛选

从全部展开节点中，根据 `relay_nodes` 规则筛选出要写入配置文件的真实节点。

### 3. `subscriptions` 订阅过滤

在 outbounds 模块中，`subscriptions` 字段过滤哪些订阅的节点会被写入此模块文件。过滤规则：
- 普通节点：检查节点 tag 是否包含指定的订阅名（如 `[sub_a]`）
- Relay 节点：检查节点 tag 是否包含指定的订阅名（检查 detour 部分，如 `[relay_x] xxx [sub_a] xxx`）

### 4. `include_relay_nodes` 逐 selector 筛选

在每个 `selectors[]` 项中，`include_relay_nodes` 作为 include 关键词列表，决定哪些已写入的 relay 节点 tag 出现在该 selector 的 outbounds 中。

---

## `modules` — 模块定义

定义 sing-box 配置文件的各个模块片段。模块可以从本地文件或远程 URL 加载，用于组装最终的 sing-box 配置。

```json
{
  "modules": {
    "log":          [ ... ],
    "dns":          [ ... ],
    "ntp":          [ ... ],
    "certificate":  [ ... ],
    "endpoints":    [ ... ],
    "inbounds":     [ ... ],
    "outbounds":    [ ... ],
    "route":        [ ... ],
    "services":     [ ... ],
    "experimental": [ ... ]
  }
}
```

每种类型下可定义多个模块，每个模块的结构如下：

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | ✅ | 模块唯一名称，在 `configs` 中引用 |
| `path` | string | ⚠️ | 本地 JSON 文件路径，与 `from_url` 二选一 |
| `from_url` | string | ⚠️ | 远程 JSON URL，与 `path` 二选一 |

模块类型与 sing-box 配置结构一一映射：

| 模块类型 | 对应 sing-box 字段 | 说明 |
|----------|-------------------|------|
| `log` | `log` | 日志配置 |
| `dns` | `dns` | DNS 配置 |
| `ntp` | `ntp` | NTP 时间同步 |
| `certificate` | `certificate` | 证书配置 |
| `endpoints` | `endpoints` | 端点配置 |
| `inbounds` | `inbounds` | 入站配置 |
| `outbounds` | `outbounds` | 出站配置（支持节点写入和 selector 更新） |
| `route` | `route` | 路由规则 |
| `services` | `services` | 服务配置 |
| `experimental` | `experimental` | 实验性功能 |

### `modules.outbounds` — 特殊字段

outbounds 模块支持额外的字段用于节点管理：

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `subscriptions` | string[] | ❌ | 指定使用的订阅名称列表，用于过滤写入此模块的节点 |
| `selectors` | Selector[] | ❌ | 定义 selector 更新规则列表 |

### `modules.outbounds[].selectors` — Selector 更新规则

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `insert_marker` | string | ✅ | 目标 selector 的 tag 名称 |
| `include_nodes` | string[] | ❌ | 包含关键词过滤。只有 tag 包含列表中任一关键词的节点会被加入此 selector |
| `exclude_nodes` | string[] | ❌ | 排除关键词过滤。tag 包含列表中任一关键词的节点会被从此 selector 中排除 |
| `include_relay_nodes` | string[] | ❌ | Relay 节点过滤关键词。决定哪些 relay 展开后的节点 tag 会出现在此 selector 中 |

**示例：**

```json
{
  "modules": {
    "dns": [
      {
        "name": "cloudflare_dns",
        "from_url": "https://example.com/modules/dns/cloudflare.json"
      },
      {
        "name": "local_dns",
        "path": "./modules/dns.json"
      }
    ],
    "outbounds": [
      {
        "name": "main_outbound",
        "path": "./modules/outbounds/main.json",
        "subscriptions": ["clash_subscription_1", "singbox_subscription_1"],
        "selectors": [
          {
            "insert_marker": "proxy-selector",
            "include_nodes": ["香港", "新加坡", "美国"],
            "exclude_nodes": ["过期", "测试"],
            "include_relay_nodes": ["relay-hk-01", "relay-sg-01"]
          },
          {
            "insert_marker": "streaming-selector",
            "include_nodes": ["Netflix", "Disney"]
          }
        ]
      },
      {
        "name": "block_outbound",
        "from_url": "https://example.com/modules/outbounds/block.json"
      }
    ],
    "route": [
      {
        "name": "china_route",
        "from_url": "https://example.com/modules/routes/china.json"
      }
    ]
  }
}
```

---

## `configs` — 配置文件合成

定义如何将多个模块组装成最终的 sing-box 配置文件。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | ✅ | 配置名称 |
| `path` | string | ✅ | 输出文件路径 |
| `modules` | string[] | ✅ | 要应用的模块名称列表（必须在 `modules` 中定义） |
| `no_need_nodes` | string[] | ❌ | 后处理关键词列表。合成后，outbounds 和 endpoints 中 tag 包含这些关键词的节点会被移除 |

在合成过程中，程序还会自动执行以下后处理：
- 将 `wireguard` 和 `tailscale` 类型的 outbound 移动到 `endpoints`
- 清除 endpoints 中带有方括号 `[]` 前缀的订阅节点
- 根据 `no_need` 关键词过滤不需要的节点
- 移除所有空的模块字段

**示例：**

```json
{
  "configs": [
    {
      "name": "main_config",
      "path": "./singbox/config.json",
      "modules": [
        "default_log",
        "cloudflare_dns",
        "direct_outbound",
        "china_route"
      ],
      "no_need_nodes": ["测试", "过期", "失效"]
    }
  ]
}
```

---

## `update_schedule` — 更新调度

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `type` | string | ✅ | 调度模式：`"interval"` 或 `"hourly"` |
| `interval` | int | ⚠️ | 更新间隔（小时），仅在 `type` 为 `"interval"` 时必填 |

**调度模式：**

| 模式 | 行为 | 配置方式 |
|------|------|----------|
| `interval` | 启动后立即执行，然后按固定小时间隔循环 | `{"type": "interval", "interval": 6}` |
| `hourly` | 启动后立即执行，然后在每个整点执行 | `{"type": "hourly"}` |

程序在后台运行时会监测配置文件变化（MD5 哈希 + 修改时间），变化时自动重载配置并执行更新。

---

## `log_level` — 日志级别

| 值 | 说明 |
|------|------|
| `"silent"` | 不输出任何日志 |
| `"error"` | 仅输出错误信息 |
| `"warn"` | 输出警告和错误 |
| `"info"` | 输出信息、警告和错误（默认） |
| `"debug"` | 输出所有日志，包括调试信息 |

也可以通过环境变量 `LOG_LEVEL` 设置。配置文件中的设置优先级更高。

---

## `proxy` — 代理设置

可选。配置后所有 HTTP 请求（订阅获取、模块下载）都会通过此代理。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `type` | string | ✅ | 代理类型：`"http"` / `"https"` / `"socks5"` |
| `host` | string | ✅ | 代理服务器地址 |
| `port` | int | ✅ | 代理服务器端口（1–65535） |
| `username` | string | ❌ | 认证用户名 |
| `password` | string | ❌ | 认证密码 |

不配置 `proxy` 时使用直连。

```json
{
  "proxy": {
    "type": "http",
    "host": "127.0.0.1",
    "port": 7890,
    "username": "user",
    "password": "pass"
  }
}
```

---

## `user_agent` — 全局 User-Agent

可选。设置 HTTP 请求的 User-Agent 头。

**优先级（从高到低）：**

1. 订阅的 `user_agent` 字段
2. 全局 `user_agent` 字段
3. 默认值 `"sing-box"`

```json
{
  "user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
}
```

---

## 环境变量

| 变量名 | 说明 |
|--------|------|
| `NODE_BOX_CONFIG` | 配置文件路径（优先级低于命令行参数，高于默认路径） |
| `LOG_LEVEL` | 日志级别，当配置文件未设置 `log_level` 时生效 |
| `LOG_TIME` | 设为 `true` 或 `1` 启用日志时间戳 |
| `LOG_PREFIX` | 日志前缀字符串 |

---

## 完整配置示例

参见 [example.json](./example.json)。
