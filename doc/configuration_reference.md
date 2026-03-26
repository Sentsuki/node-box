# 配置文件参考手册

node-box 使用 JSON 格式的配置文件。可通过命令 `node-box init config.json` 生成示例配置。

---

## 顶层结构

```json
{
  "nodes":           { ... },   // 必填 - 节点订阅与全局过滤相关设置
  "modules":         { ... },   // 可选 - sing-box 各个模块的定义
  "configs":         [ ... ],   // 可选 - 配置文件模块组装合成规则
  "update_schedule": { ... },   // 必填 - 更新与轮询调度设置
  "log_level":       "info",    // 可选 - 日志级别
  "proxy":           { ... },   // 可选 - HTTP 代理设置
  "user_agent":      "..."      // 可选 - 全局默认请求 User-Agent
}
```

---

## `nodes` — 节点配置

负责配置节点订阅源和全局范围生效的过滤规则。

### `nodes.subscriptions` — 订阅源列表

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | ✅ | 订阅名称，节点 tag 会以此作为前缀（如 `[name] 节点名`） |
| `url` | string | ⚠️ | 远程订阅 URL（与 `path` 二选一） |
| `path` | string | ⚠️ | 本地订阅文件路径（与 `url` 二选一） |
| `type` | string | ✅ | 订阅类型，可选：`"clash"`, `"singbox"`, `"relay"` |
| `enable` | bool | ✅ | 是否启用该订阅 |
| `emoji` | bool | ❌ | 控制节点名称中的 Emoji 处理方式。不填：保留订阅源原始格式；`true`：根据节点名自动适配 Emoji（移除原有 Emoji 并按地区关键词重新分配）；`false`：移除所有 Emoji |
| `remove_keywords` | string[] | ❌ | 从节点名称中移除的关键词列表。支持 `*` (匹配任意字符) 和 `?` (匹配单个字符) 通配符。 |
| `user_agent` | string | ❌ | 请求当前订阅专用 User-Agent（优先级最高） |

### `nodes.exclude_keywords` — 全局排除关键词

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `exclude_keywords` | string[] | ❌ | 节点 tag 中包含此列表任一关键词，则该节点会被全局忽略，不参与后续任何处理 |

### `nodes.relay_nodes` — 中继节点生成规则

仅当包含 `type: "relay"` 系列订阅时生效，控制中继节点（链式代理）的生成与过滤规则。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `tag` | string | ✅ | relay 模板节点的 tag 中必须包含此关键词 |
| `upstream` | string[] | ✅ | 普通节点的 tag 中必须包含此列表任一关键词 |

**注**：只有同时满足上述两个条件的节点组合，才会生成对应的中继节点。

---

## `modules` — 模块定义

用于声明 sing-box 的各类配置模块片段（来源可以是本地或网络），供后续 `configs` 组装使用。

支持的模块类型对应 sing-box 的标准配置结构：`log`, `dns`, `ntp`, `certificate`, `certificate_providers`, `endpoints`, `inbounds`, `outbounds`, `route`, `services`, `experimental`。

### 通用模块参数

所有模块的基础参数如下：

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | ✅ | 模块唯一引用标识 |
| `path` | string | ⚠️ | 本地 JSON 文件路径（与 `from_url` 二选一） |
| `from_url` | string | ⚠️ | 远程 JSON 文件链接（与 `path` 二选一） |

### `modules.outbounds` — 出站模块的特殊参数

如果指定了 `path` 指向本地文件，`outbounds` 额外支持下列用于将节点“写入并更新”到对应文件的参数配置：

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `subscriptions` | string[] | ❌ | 筛选要写入此模块文件的订阅源名称列表。为空或不填则默认写入**所有**提取的订阅节点 |
| `selectors` | Selector[] | ❌ | 将已写入的节点分配到指定的 Selector (出站选择器) 下进行更新 |

### `modules.outbounds[].selectors` — 选路器更新规则

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `insert_marker` | string | ✅ | 对应文件中，作为容器的目标出站节点的 `tag` 名称（支持 `selector` 和 `urltest` 类型） |
| `include_nodes` | string[] | ❌ | 包含词筛选。节点 tag 匹配任一元素即被抓取进入该 selector |
| `exclude_nodes` | string[] | ❌ | 排除词筛选。优先级高，节点 tag 匹配任一元素便会在此 selector 中出局 |
| `include_relay_nodes` | string[] | ❌ | 针对已生成的 Relay 节点的专用筛选，tag 匹配的会被装载进入此 selector |

---

## `configs` — 配置文件合成组装

将前文 `modules` 内声明的多个模块片段，组合打包为一份完整的 sing-box 配置文件。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | ✅ | 组装策略名 |
| `path` | string | ✅ | 汇编合并后完整配置文件的输出路径 |
| `modules` | string[] | ✅ | 要包含在组合内的方法模块名 `name` 集合 |
| `no_need_nodes` | string[] | ❌ | 合成后的清理关键词。会在合并完成时查验 outputs/endpoints，将 tag 命中的节点彻底剔除 |

---

## `update_schedule` — 更新调度设置

控制配置拉取与合成自动刷新的周期。即使配置不触发调度，对 config.json 的文件修改也会实时被捕获并主动重载。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `type` | string | ✅ | 调度模式。可选： `"interval"` (固定时隔), `"hourly"` (系统时钟整点) |
| `interval` | int | ⚠️ | 间隔小时数。仅当 `type` 为 `"interval"` 时有效并要求填写 |

---

## 附加功能设置

### `proxy` — 网络代理

开启后，系统在向外部订阅获取与拉取远程模块配置时，将会走该配置的 HTTP 协议代理。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `type` | string | ✅ | 协议类型：`"http"`, `"https"`, `"socks5"` |
| `host` | string | ✅ | 代理服务器 IP 或域名 |
| `port` | int | ✅ | 代理服务器端口号 |
| `username` | string | ❌ | (可选) 代理鉴权账户 |
| `password` | string | ❌ | (可选) 代理鉴权密码 |

### `log_level` — 日志输出等级

控制控制台或日志的打印详情级别。支持填入 `"silent"`, `"error"`, `"warn"`, `"info"`, `"debug"` (默认 `"info"`)。

### `user_agent` — 全局 User-Agent 伪装

为程序发出 HTTP 请求默认附加指定的浏览客户类型标识（兜底为 `"sing-box"`）。单条 `nodes.subscriptions` 配置下设的将优先覆盖此项。

---

## 核心支持环境变量

当不想全量依赖配置文件时，可以通过设置下面的环境标量达成控制改动目的。

* **`NODE_BOX_CONFIG`**: 用于强行覆盖读取默认配置文件路径的入参
* **`LOG_LEVEL`**: 设定初始化运行未读配置文件前（或者未设定时）的全局打印门槛
* **`LOG_TIME`**: 取值为 `true` 或者 `1`，激活并每条打印带有时区的精确日历钟表节点
* **`LOG_PREFIX`**: 主动加上特定打印文本的前缀标签用于日志清洗过滤分拣
