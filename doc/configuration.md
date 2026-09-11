# 配置参考

> 本文描述 node-box 重构后的配置格式。与旧版本**不兼容**，按全新安装编写。
> outbounds / endpoints 相关字段标注为 **【待定】**，当前阶段不实现。

## 1. 两层配置

配置分成两层，因为"怎么去 GitHub 拿配置"这件事本身不能放在 GitHub 上。

| | 本地引导配置 | 仓库配置 |
|---|---|---|
| 路径 | `/opt/node-box/node-box.json` | 配置仓库根目录的 `config.json` |
| 内容 | 去哪拿配置、监听什么端口、状态目录 | 订阅、模块、产出规则、更新周期 |
| 变更频率 | 极少，手工维护 | 经常，通过 git 管理 |
| 是否进 git | 否 | 是 |

两个文件**不要同名**——本地的叫 `node-box.json`，仓库的叫 `config.json`。

引导配置的路径解析顺序，从高到低：

1. `--config <path>` 命令行参数
2. `NODE_BOX_CONFIG` 环境变量
3. 默认值 `./node-box.json`（相对于二进制所在目录）

---

## 2. 本地引导配置 `node-box.json`

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

### 顶层字段

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|:---:|---|---|
| `root` | string | ❌ | `node-box.json` 所在目录 | 状态根目录，`snapshots/`、`state/`、`out/` 都在其下 |
| `log_level` | string | ❌ | `info` | `silent` / `error` / `warn` / `info` / `debug` |
| `source` | object | ✅ | — | 配置来源 |
| `server` | object | ❌ | 不启用 | 内置 HTTP server |

### `source`

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|:---:|---|---|
| `type` | string | ✅ | — | `github` 或 `local` |
| `repo` | string | ⚠️ | — | `type=github` 时必填，格式 `owner/name` |
| `branch` | string | ❌ | `main` | 分支名 |
| `token_env` | string | ⚠️ | — | `type=github` 时必填。**存的是环境变量名，不是 token 本身** |
| `poll_interval` | string | ❌ | `10m` | 兜底轮询间隔，Go duration 格式。设为 `"0"` 关闭（不建议） |
| `dir` | string | ⚠️ | — | `type=local` 时必填，本地配置目录路径 |

`type: "local"` 用于本地开发和调试，直接读一个目录，不走网络：

```json
"source": { "type": "local", "dir": "/home/me/node-box-config" }
```

### `server`

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|:---:|---|---|
| `enabled` | bool | ❌ | `false` | 是否启动内置 HTTP server |
| `listen` | string | ⚠️ | `127.0.0.1:8788` | 监听地址。**应始终绑定回环地址**，TLS 交给前置反代 |
| `webhook_secret_env` | string | ⚠️ | — | `enabled=true` 时必填，HMAC 密钥的环境变量名 |

### `.env`

两个密钥放在 `/opt/node-box/.env`，权限 `0600`，由 systemd 的 `EnvironmentFile` 注入：

```sh
NODE_BOX_GH_TOKEN=github_pat_xxxxxxxxxxxx
NODE_BOX_WEBHOOK_SECRET=<openssl rand -hex 32 生成>
```

GitHub token 用 fine-grained PAT，权限只需要目标仓库的 **Contents: Read-only**。

---

## 3. 仓库配置 `config.json`

```json
{
  "output": {
    "dir": "out"
  },

  "nodes": {
    "subscriptions": [
      {
        "name": "airport-a",
        "url": "https://example.com/sub?token=xxx",
        "type": "clash",
        "enable": true,
        "emoji": true,
        "remove_keywords": ["(*人)", "BGP专线"],
        "user_agent": "sing-box/1.10.0"
      }
    ],
    "exclude_keywords": ["过期", "失效", "剩余流量"]
  },

  "modules": [
    { "name": "log",      "file": "modules/log.json" },
    { "name": "dns",      "file": "modules/dns.json" },
    { "name": "route",    "file": "modules/route.json" },
    { "name": "inbounds", "file": "modules/inbounds.json" },
    { "name": "geoip",    "from_url": "https://example.com/shared/geoip.json" }
  ],

  "configs": [
    {
      "name": "main",
      "path": "main.json",
      "modules": ["log", "dns", "route", "inbounds"]
    },
    {
      "name": "vps2",
      "path": "/etc/sing-box/config.json",
      "modules": ["log", "dns", "route", "inbounds", "geoip"]
    }
  ],

  "update_schedule": {
    "type": "interval",
    "every": "6h"
  }
}
```

### 顶层字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `output` | object | ❌ | 产出路径设置 |
| `nodes` | object | ✅ | 订阅源与全局过滤 |
| `modules` | array | ✅ | 模块列表（**扁平，不再按类型分组**） |
| `configs` | array | ✅ | 产出文件的组装规则 |
| `update_schedule` | object | ✅ | 定时重抓订阅的周期 |
| `user_agent` | string | ❌ | 全局默认 User-Agent，默认 `sing-box` |

---

### 3.1 `output`

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|:---:|---|---|
| `dir` | string | ❌ | `<root>/out` | 产出根目录。相对路径相对于 `root` 解析 |

**产出路径的解析规则**（`configs[].path`）：

- **绝对路径** → 直接使用，忽略 `output.dir`
- **相对路径** → 相对 `output.dir` 解析

```jsonc
"output": { "dir": "out" },              // 展开为 /opt/node-box/out
"configs": [
  { "path": "main.json" },               // → /opt/node-box/out/main.json
  { "path": "sub/gaming.json" },         // → /opt/node-box/out/sub/gaming.json
  { "path": "/etc/sing-box/config.json" }// → /etc/sing-box/config.json
]
```

启动时的校验（不等到写盘那一刻才失败）：

- `output.dir` 下的目标目录不存在 → 自动 `MkdirAll`
- **绝对路径**的目标目录不存在 → **报错，不自动创建**（路径打错导致默默造出垃圾目录，比报错难查得多）
- 目标目录不可写 → 报错
- 两个 `configs` 解析后指向同一个文件 → 报错
- 产出路径落在 `snapshots/` 内 → 报错（会污染只读快照）

产出文件权限 `0600`。

---

### 3.2 `nodes.subscriptions`

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `name` | string | ✅ | 订阅名，唯一。节点 tag 会加 `[name] ` 前缀 |
| `url` | string | ⚠️ | 远程订阅地址，与 `path` 二选一 |
| `path` | string | ⚠️ | 本地订阅文件，与 `url` 二选一。相对路径相对快照根目录解析 |
| `type` | string | ✅ | `clash` / `singbox` / `xray` / `v2ray` / `relay`【待定】 |
| `enable` | bool | ✅ | 是否启用 |
| `emoji` | bool | ❌ | 不填：保留原样；`true`：按地区关键词重新分配；`false`：移除全部 emoji |
| `remove_keywords` | string[] | ❌ | 从节点名中移除的关键词，支持 `*` 和 `?` 通配符 |
| `user_agent` | string | ❌ | 该订阅专用 UA，优先级高于全局 `user_agent` |

校验规则：

- `name` 不能为空、不能重复
- `name` 不能包含 `[` 或 `]`（会与 tag 前缀机制冲突）
- `url` 与 `path` 必须且只能有一个

### 3.3 `nodes.exclude_keywords`

字符串数组。节点 tag 命中任一关键词则该节点被全局丢弃，不参与后续任何处理。
比较时忽略双方的 emoji。

### 3.4 `nodes.relay_nodes`【待定】

中继节点生成规则，随 outbounds 一并重新设计。

---

### 3.5 `modules`

**扁平列表，不再按 `log` / `dns` / `route` 等类型分组。**

分组在旧版本里是纯装饰——代码从不读它，模块实际生效的是文件内容里的顶层键。
`config.json` 里声明它是 `dns` 而文件里写的是 `route`，以文件为准。
两处真相源只会造成误解，所以去掉。

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `name` | string | ✅ | 模块唯一标识，供 `configs[].modules` 引用 |
| `file` | string | ⚠️ | 仓库内相对路径，三选一 |
| `from_url` | string | ⚠️ | 外部 JSON 地址，三选一。用于引用**他人维护的**模块 |
| `path` | string | ⚠️ | 运行机器上的绝对路径，三选一。仅用于本地开发 |
| `selectors` | array | ❌ | 【待定】节点注入规则 |
| `subscriptions` | string[] | ❌ | 【待定】限定注入哪些订阅的节点 |

校验规则：

- `name` 不能为空、不能重复
- `file` / `from_url` / `path` 必须且只能有一个
- `file` 指向的路径必须在快照内存在，且不能逃逸出快照根目录（禁止 `../`）

---

### 3.6 `configs`

每一项描述一个产出文件。

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `name` | string | ✅ | 产出名，唯一，用于日志和错误信息 |
| `path` | string | ✅ | 产出路径，解析规则见 3.1 |
| `modules` | string[] | ✅ | 要组装的模块名，**按此顺序合并** |
| `no_need_nodes` | string[] | ❌ | 【待定】产出级节点过滤 |

校验规则：

- `name` / `path` 不能为空，`name` 不能重复
- `modules` 不能为空，且每一项必须能在 `modules` 中找到

---

### 3.7 `update_schedule`

控制**定时重抓订阅**的周期。与 GitHub 配置变更的触发是两回事，详见 `architecture.md` 第 6 节。

```json
{ "type": "interval", "every": "6h" }
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `type` | string | ✅ | `interval` 或 `hourly` |
| `every` | string | ⚠️ | `type=interval` 时必填。Go duration 格式：`"30m"` / `"6h"` / `"24h"` |

- `interval` —— 每隔 `every` 执行一次，从进程启动开始计时
- `hourly` —— 在每个整点执行

`every` 不再是旧版的整数小时，改成 duration 字符串，与 `poll_interval` 保持一致，
也支持小于 1 小时的周期。

---

## 4. 模块文件格式

模块文件是一个 **JSON 对象**，它的顶层键会被直接合并进产出配置。

`modules/log.json`：

```json
{
  "log": {
    "level": "info",
    "timestamp": true
  }
}
```

`modules/dns.json`：

```json
{
  "dns": {
    "servers": [ ... ],
    "rules":   [ ... ]
  }
}
```

一个模块文件里可以定义多个顶层键：

```json
{
  "ntp":  { "enabled": true, "server": "time.apple.com" },
  "log":  { "level": "warn" }
}
```

### 合并语义

1. 按 `configs[].modules` 列出的**顺序**依次合并
2. 合并的是**顶层键**，不做深合并
3. **顶层键冲突 → 报错**，错误信息指出是哪两个模块争抢哪个键

第 3 条是与旧版本的重要区别。旧版是静默 last-wins——两个模块都定义 `route` 时你会丢掉
一整块配置且毫无提示。现在这会直接失败。

如果确实想让 B 覆盖 A，就不要同时引用两者，或者把它们合并成一个模块文件。

### 空字段清理

序列化之前，删除所有值为 `null`、`[]`、`{}` 的顶层键。

这是一条**通用规则**，不依赖任何字段白名单——不管键叫什么，空的就删掉。
（旧版本硬编码了一份 12 个字段名的列表，与模块类型定义重复，是第二处真相源。）

### 模块文件的校验

- 必须是合法 JSON
- 顶层必须是对象（不能是数组或标量）
- 单个模块文件上限 8 MB

---

## 5. 完整示例

配置仓库：

```
node-box-config/
├── config.json
├── modules/
│   ├── log.json
│   ├── dns.json
│   ├── route.json
│   └── inbounds.json
└── .github/workflows/notify.yml
```

`config.json`：

```json
{
  "output": { "dir": "out" },

  "nodes": {
    "subscriptions": [
      {
        "name": "airport-a",
        "url": "https://a.example.com/link/xxxx?clash=1",
        "type": "clash",
        "enable": true,
        "emoji": true,
        "remove_keywords": ["(*人)"]
      },
      {
        "name": "airport-b",
        "url": "https://b.example.com/sub/yyyy",
        "type": "singbox",
        "enable": true
      }
    ],
    "exclude_keywords": ["过期", "官网", "剩余"]
  },

  "modules": [
    { "name": "log",      "file": "modules/log.json" },
    { "name": "dns",      "file": "modules/dns.json" },
    { "name": "route",    "file": "modules/route.json" },
    { "name": "inbounds", "file": "modules/inbounds.json" }
  ],

  "configs": [
    {
      "name": "main",
      "path": "main.json",
      "modules": ["log", "dns", "inbounds", "route"]
    }
  ],

  "update_schedule": { "type": "interval", "every": "6h" }
}
```

产出 `/opt/node-box/out/main.json`：

```json
{
  "dns": { ... },
  "inbounds": [ ... ],
  "log": { ... },
  "route": { ... }
}
```

顶层键按字典序排列（`encoding/json` 对 map 的行为），所以产出是确定性的，
内容没变时 hash 也不会变，写盘会被跳过。
