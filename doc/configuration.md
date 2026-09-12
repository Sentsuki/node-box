# 配置说明

node-box 包含两个配置文件：

| | 本地引导配置 | 仓库配置 |
|---|---|---|
| 文件名 | `node-box.json` | `config.json` |
| 位置 | 运行 node-box 的主机环境 | 配置仓库根目录 |
| 内容 | 仓库来源、HTTP 监听、状态目录 | 订阅源、模块、产出规则、更新周期 |
| 变更频率 | 极少 | 经常（通过 Git 管理） |

完整示例可参考 [`example.json`](example.json)。

> [!IMPORTANT]
> 所有配置文件均拒绝未知字段。键名错误会在解析时直接报错退出。

---

# 一、本地引导配置 `node-box.json`

```json
{
  "root": "/opt/node-box",
  "log_level": "info",
  "source": {
    "type": "github",
    "repo": "you/node-box-config",
    "branch": "main",
    "token_env": "NODE_BOX_GH_TOKEN",
    "poll_interval": "10m"
  },
  "server": {
    "enabled": true,
    "listen": "127.0.0.1:8788",
    "webhook_secret_env": "NODE_BOX_WEBHOOK_SECRET"
  }
}
```

配置文件路径优先级：`--config <路径>` > `NODE_BOX_CONFIG` 环境变量 > 程序同目录下的 `node-box.json`。

## 顶层字段

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|---|---|:---:|---|---|
| `root` | string | ❌ | `node-box.json` 所在目录 | 状态根目录，包含 `snapshots/`、`state/`、`out/` |
| `log_level` | string | ❌ | `info` | 日志级别：`silent` / `error` / `warn` / `info` / `debug` |
| `update_timeout` | duration | ❌ | `15m` | 单次更新超时上限，超时则中止本次更新并记入 `state.json` |
| `source` | object | ✅ | — | 配置来源设置 |
| `server` | object | ❌ | 不启用 | 内置 HTTP 服务 |
| `proxy` | object | ❌ | 直连 | 出站代理，对拉取配置仓库和拉取订阅均生效 |

## `source`

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|---|---|:---:|---|---|
| `type` | string | ✅ | — | `github` 或 `local` |
| `repo` | string | ⚠️ | — | `type=github` 必填，格式为 `owner/name` |
| `branch` | string | ❌ | `main` | 分支名 |
| `token_env` | string | ⚠️ | — | `type=github` 必填，存储 GitHub Token 的环境变量名 |
| `poll_interval` | string | ❌ | `10m` | 兜底轮询间隔（Go duration 格式），`"0"` 表示关闭 |
| `dir` | string | ⚠️ | — | `type=local` 必填，本地配置目录路径 |

- **GitHub 权限**：Token 使用 Fine-grained PAT 时，仅需目标仓库的 **Contents: Read-only** 权限。
- **本地源模式**：`type: "local"` 用于本地开发调试，直接读取目录内容，以内容哈希作为版本标识。

## `server`

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|---|---|:---:|---|---|
| `enabled` | bool | ❌ | `false` | 是否启用内置 HTTP 服务 |
| `listen` | string | ❌ | `127.0.0.1:8788` | **仅允许绑定回环地址**，填 `0.0.0.0` 或 `::` 将拒绝启动 |
| `webhook_secret_env` | string | ⚠️ | — | `enabled=true` 必填，HMAC 密钥的环境变量名 |

HTTP 端点列表：

| 端点 | 鉴权 | 说明 |
|---|---|---|
| `POST /hooks/github` | HMAC-SHA256 (`X-NodeBox-Signature-256`) | GitHub Webhook 触发入口，使用密钥恒定时间校验原始请求体 |
| `GET /healthz` | 无 | 健康检查，返回 `{"status":"ok"}` |
| `GET /status` | 无 | 返回当前/上一个快照 ref、产出路径及哈希、上次更新错误、运行状态 |

> [!NOTE]
> node-box 不提供 TLS 及针对 `/status` 的二次鉴权。如需公网访问或权限隔离，请前置反向代理（如 Caddy/Nginx）。

## `proxy`

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `type` | string | ✅ | `http` / `https` / `socks5` |
| `host` | string | ✅ | 代理服务器地址 |
| `port` | int | ✅ | 端口号 (1–65535) |
| `username` | string | ❌ | 认证用户名 |
| `password` | string | ❌ | 认证密码 |

## 环境变量凭据

敏感密钥推荐保存在权限为 `0600` 的 `.env` 文件中，通过 systemd `EnvironmentFile` 等机制注入：

```sh
NODE_BOX_GH_TOKEN=github_pat_xxxxxxxxxxxx
NODE_BOX_WEBHOOK_SECRET=<openssl rand -hex 32>
```

---

# 二、仓库配置 `config.json`

## 顶层字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `output` | object | ❌ | 产出路径配置 |
| `nodes` | object | ✅ | 订阅源、全局过滤与中继规则 |
| `modules` | array | ✅ | 模块列表 |
| `configs` | array | ✅ | 产出文件组装定义 |
| `update_schedule` | object | ✅ | 定时重抓订阅周期 |
| `user_agent` | string | ❌ | 全局默认 User-Agent，默认为 `sing-box` |

---

## `output`

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|---|---|:---:|---|---|
| `dir` | string | ❌ | `<root>/out` | 产出根目录；相对路径相对于 `root` 解析 |

路径解析与校验规则：
- **绝对路径**：直接使用，忽略 `output.dir`。
- **相对路径**：相对于 `output.dir` 解析。
- **目录校验**：`output.dir` 目录不存在会自动创建；绝对路径的目标目录若不存在则直接报错；目标目录不可写或两个产出路径冲突均直接报错；产出路径禁止落在 `snapshots/` 内。
- **写入行为**：文件权限为 `0600`，采用原子写入（临时文件 + 重命名），写入前比对内容，一致则跳过。

---

## `nodes.subscriptions`

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `name` | string | ✅ | 订阅唯一标识，节点 tag 会添加 `[name] ` 前缀（不能包含 `[` 或 `]`） |
| `url` | string | ⚠️ | 远程订阅地址，与 `path` 二选一 |
| `path` | string | ⚠️ | 仓库内相对路径，与 `url` 二选一 |
| `type` | string | ✅ | 格式类型：`clash` / `singbox` / `xray` / `v2ray` |
| `enable` | bool | ✅ | 是否启用 |
| `emoji` | bool | ❌ | 不填保留原样；`true` 移除原有并按地区关键词重分；`false` 仅移除 |
| `remove_keywords` | string[] | ❌ | 从节点名移除的关键词（支持 `*` 和 `?` 通配符） |
| `user_agent` | string | ❌ | 该订阅专用的 User-Agent（优先于全局配置） |

- **处理顺序**：解析 → `remove_keywords` → `emoji` → 添加 `[name] ` 前缀 → `exclude_keywords`。
- **Endpoint 节点**：`clash` 订阅中的 WireGuard / OpenVPN 节点将转换为 endpoint 并自动归入产出的 `endpoints` 段；`singbox` 订阅会同时读取 `outbounds` 与 `endpoints`。

---

## `nodes.exclude_keywords`

字符串数组。节点 tag 命中任意关键词则在抓取阶段直接丢弃（常用于清洗流量信息、到期提醒等非节点条目）。匹配时忽略两端 emoji。

---

## `nodes.emoji_overrides`

自定义地区国旗匹配规则，优先级高于内置映射表：

```json
"nodes": {
  "emoji_overrides": [
    { "emoji": "🇱🇺", "keywords": ["卢森堡", "LU", "Luxembourg"] },
    { "emoji": "🏴󠁧󠁢󠁳󠁣󠁴󠁿", "keywords": ["苏格兰", "Scotland"] }
  ]
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `emoji` | string | ✅ | 匹配后赋予的 emoji |
| `keywords` | string[] | ✅ | 地区关键词列表（不区分大小写，整词匹配） |

- 按列表顺序优先匹配，首个命中生效；未命中任何规则默认贴 🇺🇳。
- 关键词按整词匹配（中文以非汉字为界，英文以非字母为界）。

---

## NodeSelector（节点选择器）

统一用于 selector 成员配置及中继的 `via` / `upstream`：

| 字段 | 类型 | 说明 |
|---|---|---|
| `from` | string[] | 订阅名称列表。**留空表示不选取任何普通节点** |
| `include` | string[] | 节点 tag 包含任一关键词才保留；留空表示保留全部 |
| `exclude` | string[] | 节点 tag 包含任一关键词则丢弃 |

- `include` / `exclude` 匹配时均忽略 emoji。

---

## `nodes.relays`（中继节点）

将 `via`（模板节点）与 `upstream`（上游节点）两两组合，生成 `detour` 指向上游的中继节点。

```json
{
  "name": "us",
  "via": [{ "from": ["RL"], "include": ["US"] }],
  "upstream": [
    { "from": ["airport-a"], "include": ["美国"] },
    { "from": ["airport-b"], "include": ["香港"] }
  ]
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `name` | string | ✅ | 唯一标识，供 `selectors[].relays` 引用 |
| `via` | NodeSelector[] | ✅ | 模板节点选择器，数组元素间取并集 |
| `upstream` | NodeSelector[] | ✅ | 上游节点选择器，数组元素间取并集 |

- 生成的节点 tag 格式为 `{模板tag} {上游tag}`。
- `via` 或 `upstream` 匹配结果为 0 时将报错。
- 重复配对自动去重，保留声明靠前的位置。

---

## `modules`

定义可复用的 sing-box JSON 配置片段：

```json
{ "log": { "level": "info", "timestamp": true } }
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `name` | string | ✅ | 唯一标识，供 `configs[].modules` 引用 |
| `file` | string | ⚠️ | 仓库内相对路径，三选一 |
| `from_url` | string | ⚠️ | 外部 JSON URL，三选一 |
| `path` | string | ⚠️ | 本机绝对路径（本地调试使用），三选一 |
| `selectors` | array | ❌ | 节点注入规则 |

合并规则：
1. 模块文件必须为 JSON 对象。
2. 按照 `configs[].modules` 中声明的顺序合并顶层键（仅顶层合并，不做深合并）。
3. **顶层键冲突将直接报错**。
4. 序列化前自动移除值为 `null`、`[]`、`{}` 的顶层键；单模块大小上限 8 MB。

---

## `modules[].selectors`

将节点注入到模块预先定义的 selector / urltest outbound 中：

```json
"selectors": [
  { "tag": "Proxy", "from": ["own", "airport-a"], "relays": ["us", "jp"] },
  { "tag": "Game",  "from": ["airport-a"], "include": ["香港"], "relays": ["jp-hk"] },
  { "tag": "AI",    "relays": ["us"] }
]
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `tag` | string | ✅ | 模块内目标 selector / urltest 的 tag |
| `from` / `include` / `exclude` | — | ❌ | 内联 NodeSelector |
| `relays` | string[] | ❌ | 引用的中继名称列表 |

- `from` 与 `relays` 至少填写一项。
- `include` / `exclude` 仅作用于 `from` 选出的普通节点，不过滤 `relays`。
- 被引用的中继上游依赖节点会自动写入产出（但不作为 selector 成员）。
- `wireguard` / `tailscale` / `openvpn-client` 节点自动归入 `endpoints`；模块自写上述类型进 `outbounds` 将报错。
- 校验：引用的 `tag` 必须存在；规则未匹配到节点将发出警告；若 selector 最终成员为空则报错。

### 节点排序规则

`outbounds` 数组与 selector 成员列表采用一致的排序策略：
1. 模块文件自带的原始条目（保留原序）
2. 中继节点（按 `nodes.relays` 声明顺序）
3. 普通节点（按 `nodes.subscriptions` 声明顺序）

---

## `configs`

产出文件定义：

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `name` | string | ✅ | 产出名称（唯一） |
| `path` | string | ✅ | 产出路径（遵循 `output` 解析规则） |
| `modules` | string[] | ✅ | 引用的模块列表（按书写顺序合并） |

校验：`name` 不可重复；`modules` 不可为空、不可重复引用、所引用的模块必须存在。

---

## `update_schedule`

配置定时重新抓取订阅节点的调度周期（与配置仓库的变更检测相互独立）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `type` | string | ✅ | `interval`（按间隔）或 `hourly`（每整点） |
| `every` | string | ⚠️ | `type=interval` 必填，Go duration 格式（如 `"30m"`、`"6h"`） |

---

# 三、产出校验

写盘前在内存中统一执行以下校验，任一项不满足均放弃写盘：
- 每个 selector / urltest 的成员非空。
- 成员与 `detour` 引用的 tag 必须在同文件内存在。
- `outbounds` 与 `endpoints` 的 tag 全局唯一。
- `outbounds` 中不包含 endpoint 专属类型。
- 每个数组段均为对象数组。
- 产出内容非空。

---

# 四、异常处理策略

| 异常情况 | 处理行为 |
|---|---|
| 拉取配置仓库失败（未指定 ref） | 回退至上一次成功产出的快照继续执行，并记录 WARN |
| 拉取配置仓库失败（指定了 ref） | 报错中止，不使用其他快照 |
| 快照解析 / 校验失败 | 报错中止，不修改任何文件 |
| 部分订阅抓取失败 | 记录 WARN，使用成功的订阅继续构建 |
| 全部订阅抓取失败 | 报错中止，不修改任何文件 |
| 组装或产出校验失败 | 报错中止，不修改任何文件，错误记入 `state.json` |
| 更新超时 (`update_timeout`) | 中止本次更新，错误记入 `state.json` |
| 写盘中途失败 | 已写入的保留，未完成的保留旧文件（单文件原子写入） |

---

# 五、并发与快照管理

- **并发控制**：写操作（后台守护进程、`update`、`rollback`）通过 `state/update.lock` 文件排他锁实现互斥；只读命令（`status`、`build`、`validate`、`pull`）可与写操作并发执行。
- **守护进程状态**（由 `status` 查看）：
  - `running, idle`：正常运行且空闲。
  - `running, update in progress`：正在执行更新。
  - `not running; the last update was interrupted before it finished`：上次更新异常中断。
- **快照指针**（位于 `snapshots/`）：
  - `current`：当前产出文件所对应的快照（仅在成功写盘后更新）。
  - `previous`：上一次生效的快照，用于 `rollback` 回滚。
