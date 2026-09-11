# 配置说明

node-box 有两个配置文件：

| | 本地引导配置 | 仓库配置 |
|---|---|---|
| 文件名 | `node-box.json` | `config.json` |
| 位置 | 运行 node-box 的机器上 | 配置仓库的根目录 |
| 内容 | 去哪拿配置、监听什么、状态目录 | 订阅、模块、产出规则、更新周期 |
| 变更频率 | 极少 | 经常，通过 git 管理 |

分两层是因为「怎么去 GitHub 拿配置」这件事本身不能放在 GitHub 上。

完整示例见 [`example.json`](example.json)（那是仓库配置）。

**所有配置文件都拒绝未知字段。** 键名写错会在启动时直接报错，不会被静默忽略。

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

路径解析顺序：`--config <路径>` > `NODE_BOX_CONFIG` 环境变量 > 二进制同目录的 `node-box.json`。

## 顶层

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|:---:|---|---|
| `root` | string | ❌ | `node-box.json` 所在目录 | 状态根目录，`snapshots/`、`state/`、`out/` 都在其下 |
| `log_level` | string | ❌ | `info` | `silent` / `error` / `warn` / `info` / `debug` |
| `source` | object | ✅ | — | 配置来源 |
| `server` | object | ❌ | 不启用 | 内置 HTTP server |
| `proxy` | object | ❌ | 直连 | 出站代理，**对拉取配置仓库和拉取订阅都生效** |

## `source`

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|:---:|---|---|
| `type` | string | ✅ | — | `github` 或 `local` |
| `repo` | string | ⚠️ | — | `type=github` 必填，格式 `owner/name` |
| `branch` | string | ❌ | `main` | 分支名 |
| `token_env` | string | ⚠️ | — | `type=github` 必填。**存的是环境变量名，不是 token 本身** |
| `poll_interval` | string | ❌ | `10m` | 兜底轮询间隔，Go duration 格式。`"0"` 关闭（不建议） |
| `dir` | string | ⚠️ | — | `type=local` 必填，本地配置目录 |

GitHub token 用 fine-grained PAT，权限只需要目标仓库的 **Contents: Read-only**。

`type: "local"` 直接读一个目录、不走网络，用于开发调试。它照样做快照和回滚，行为和
`github` 完全一致，只是「版本」是目录内容的哈希而不是 commit sha。

**为什么用仓库 tarball 而不是 raw 文件 URL**：`raw.githubusercontent.com` 有最长约
5 分钟缓存，Action 触发后可能拉到旧内容且毫无提示；而 tarball 是单个 commit 的完整
快照，不会出现「A 模块新版 + B 模块旧版」的组合。

## `server`

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|:---:|---|---|
| `enabled` | bool | ❌ | `false` | 是否启动内置 HTTP server |
| `listen` | string | ❌ | `127.0.0.1:8788` | **必须绑定回环地址**，填 `0.0.0.0` / `::` 会被拒绝启动 |
| `webhook_secret_env` | string | ⚠️ | — | `enabled=true` 必填，HMAC 密钥的环境变量名 |

TLS 交给前置的 Caddy / nginx，node-box 不管证书。三个端点：

```
POST /hooks/github     唯一写入口，需要 X-NodeBox-Signature-256
GET  /healthz          存活探测
GET  /status           当前 ref / 上次产出 / 上次错误
```

## `proxy`

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `type` | string | ✅ | `http` / `https` / `socks5` |
| `host` | string | ✅ | 代理地址 |
| `port` | int | ✅ | 1–65535 |
| `username` | string | ❌ | 认证用户名 |
| `password` | string | ❌ | 认证密码 |

## 密钥

两个密钥放在 `.env`（权限 `0600`），由 systemd 的 `EnvironmentFile` 注入：

```sh
NODE_BOX_GH_TOKEN=github_pat_xxxxxxxxxxxx
NODE_BOX_WEBHOOK_SECRET=<openssl rand -hex 32>
```

---

# 二、仓库配置 `config.json`

## 顶层

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `output` | object | ❌ | 产出路径设置 |
| `nodes` | object | ✅ | 订阅源、全局过滤、中继定义 |
| `modules` | array | ✅ | 模块列表（扁平，不按类型分组） |
| `configs` | array | ✅ | 产出文件的组装规则 |
| `update_schedule` | object | ✅ | 定时重抓订阅的周期 |
| `user_agent` | string | ❌ | 全局默认 User-Agent，默认 `sing-box` |

---

## `output`

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|:---:|---|---|
| `dir` | string | ❌ | `<root>/out` | 产出根目录。相对路径相对 `root` 解析 |

`configs[].path` 的解析规则：

- **绝对路径** → 直接使用，忽略 `output.dir`
- **相对路径** → 相对 `output.dir` 解析

启动时就校验，不等到写盘才失败：

- `output.dir` 下的目标目录不存在 → 自动创建
- **绝对路径**的目标目录不存在 → **报错，不自动创建**（路径打错默默造出垃圾目录比报错难查）
- 目标目录不可写 → 报错
- 两个 `configs` 解析到同一个文件 → 报错
- 产出路径落在 `snapshots/` 内 → 报错（会污染只读快照）

产出文件权限 `0600`。写入是原子的（同目录 tmp + `rename`），并且**先与磁盘上的
内容比对**，相同就跳过——所以手改过或被删掉的产出文件，下次运行会被恢复。

---

## `nodes.subscriptions`

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `name` | string | ✅ | 订阅名，唯一。节点 tag 会加 `[name] ` 前缀 |
| `url` | string | ⚠️ | 远程地址，与 `path` 二选一 |
| `path` | string | ⚠️ | 仓库内相对路径，与 `url` 二选一 |
| `type` | string | ✅ | `clash` / `singbox` / `xray` / `v2ray` |
| `enable` | bool | ✅ | 是否启用 |
| `emoji` | bool | ❌ | 不填：保留原样；`true`：**移除原有 emoji 再按地区关键词重新分配**；`false`：只移除 |
| `remove_keywords` | string[] | ❌ | 从节点名中移除的关键词，支持 `*` 和 `?` 通配符 |
| `user_agent` | string | ❌ | 该订阅专用 UA，优先级高于全局 `user_agent` |

处理顺序：解析 → `remove_keywords` → `emoji` → 加 `[name] ` 前缀 → `exclude_keywords`。

`clash` 订阅里的 WireGuard / OpenVPN 节点会被转换成 endpoint 类型，产出时自动写入
`endpoints` 段。`singbox` 订阅的 `outbounds` 和 `endpoints` **两个数组都会读取**。

校验：`name` 不能为空、不能重复、不能包含 `[` 或 `]`；`url` 与 `path` 必须且只能有一个。

**没有「中继订阅」这种类型。** 一个订阅的节点成为中继模板，只因为某条 `relays` 的
`via` 指向了它——身份来自引用，不来自声明。

## `nodes.exclude_keywords`

字符串数组。节点 tag 命中任一关键词则**在抓取时就被丢弃**，不进入节点池，后续任何
步骤都碰不到它。比较时忽略双方的 emoji。

这是**源头清洗**，不是选择：机场常把 `剩余流量：128GB`、`套餐到期：...` 这类东西当成
真节点塞在订阅里，它们根本不该成为节点。而「这个组不要某些节点」是 selector 规则的事。

---

## NodeSelector — 节点选择的统一形状

同一个形状用在三处：selector 成员、中继的 `via`、中继的 `upstream`。

| 字段 | 类型 | 说明 |
|---|---|---|
| `from` | string[] | 订阅名。**留空 = 不要任何普通节点** |
| `include` | string[] | tag 含其中任一关键词才保留；留空 = `from` 命中的全要 |
| `exclude` | string[] | tag 含其中任一关键词则丢弃 |

`include` / `exclude` 比较时**忽略双方的 emoji**，所以 `"香港"` 能匹配
`[mj] 🇭🇰 香港 02`。

`from` 留空**不等于「全部订阅」**。这样加新订阅不会静默改变已有规则的含义，也让中继
模板不需要任何特殊标记——没人 `from` 它，它就进不去产出。selector 用 `from` 留空来
表达「只要中继」。

---

## `nodes.relays` — 中继（链式代理）

每条声明把 `via` 选出的**模板节点**与 `upstream` 选出的**上游节点**两两配对，每对
生成一个节点，其 `detour` 指向上游。

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
| `name` | string | ✅ | 唯一标识，供 `selectors[].relays` 按名引用 |
| `via` | NodeSelector[] | ✅ | 模板节点，多个元素取**并集** |
| `upstream` | NodeSelector[] | ✅ | 上游节点，多个元素取**并集** |

生成的 tag 是 `{模板tag} {上游tag}`，例如 `[RL] US [airport-a] 🇺🇸 美国 01`。

**为什么是数组而不是单个选择**：上游经常是「A 机场的美国节点 + B 机场的香港节点」，
这是**并集**。单个 `{from:["a","b"], include:["美国","香港"]}` 是**叉积**，会多出
「A 的香港」和「B 的美国」两支。

**生成是有界的**：只有被选中的模板和上游才会配对，不存在「先全量展开再过滤」。上游有
几百个节点也不会先炸开。

**声明之间互不影响，可以重叠**：tag 由 (模板, 上游) 决定，与声明名无关。所以
`jp`（日本+香港上游）和 `jp-hk`（仅香港上游）共有的那一对在产出里只出现一次，位置归
**声明更早**的那条。一条声明是对「模板 × 上游」空间的一次**命名选择**，不是一次生成。
`jp-hk` 不会继承 `jp` 的 `exclude`，要一样就各写一遍。

`via` 或 `upstream` 匹配到 0 个节点 → **报错**。静默产出一条空中继，等于让你以为链式
代理在跑而其实没有。

---

## `modules`

**扁平列表，不按 sing-box 的段分组。** 一个模块贡献哪些段，由文件内容的顶层键决定。

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `name` | string | ✅ | 唯一标识，供 `configs[].modules` 引用 |
| `file` | string | ⚠️ | 仓库内相对路径，三选一 |
| `from_url` | string | ⚠️ | 外部 JSON 地址，三选一。用于引用**他人维护的**模块 |
| `path` | string | ⚠️ | 运行机器上的绝对路径，三选一。仅用于本地开发 |
| `selectors` | array | ❌ | selector 成员规则，见下节 |

模块文件必须是一个 **JSON 对象**，顶层键会被直接合并进产出配置：

```json
{ "log": { "level": "info", "timestamp": true } }
```

一个文件里可以定义多个顶层键。合并规则：

1. 按 `configs[].modules` 列出的**顺序**依次合并
2. 只合并**顶层键**，不做深合并
3. **顶层键冲突 → 报错**，错误信息会点出是哪两个模块争抢哪个键

第 3 条容易踩到的是 `route.json` 里顺手写了 `outbounds`（比如放个 `direct`），和真正
的 outbounds 模块撞车。检查方法：

```bash
for f in $(find modules -name '*.json'); do echo "$f: $(jq -r 'keys|join(", ")' $f)"; done
```

同一个 `configs[].modules` 列表里，所有模块的键加起来不能有重复。

序列化之前，删除所有值为 `null`、`[]`、`{}` 的顶层键。单个模块文件上限 8 MB。

---

## `modules[].selectors` — selector 成员规则

**selector 引用的节点是唯一真相。** 一个节点被写进产出，恰好因为某条规则引用了它；
没有「先插入再减掉」这一步。

```json
"selectors": [
  { "tag": "Proxy", "from": ["own", "airport-a"], "relays": ["us", "jp"] },
  { "tag": "Game",  "from": ["airport-a"], "include": ["香港"], "relays": ["jp-hk"] },
  { "tag": "AI",    "relays": ["us"] }
]
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `tag` | string | ✅ | 模块文件里那个 selector / urltest 的 tag |
| `from` / `include` / `exclude` | — | ❌ | 内联的 NodeSelector |
| `relays` | string[] | ❌ | 按名引用的中继声明 |

规则：

- 规则挂在**模块**上。该模块被哪个产出引用，规则就在那个产出里生效
- `from` 和 `relays` 至少要有一个，否则这条规则什么都不做 → 报错
- `include` / `exclude` **只作用于 `from` 选出的普通节点，不过滤 `relays`**。
  中继是按名精确引用的；想要子集就多声明一条更窄的中继
- 引用的 `tag` 在组装后的文件里不存在 → 报错
- 某条规则没匹配到任何节点 → 警告；该 selector 最终成员为空 → **报错**

**自动带入 detour 依赖**：一条规则只引用中继时，中继的上游节点也会被写进产出（但不会
成为 selector 成员）。否则产出里会是一堆悬空的 detour。

**`wireguard` / `tailscale` / `openvpn-client` 类型的节点写入 `endpoints`**，selector
仍然按 tag 引用它们。反过来，模块文件自己在 `outbounds` 里手写这些类型 → **报错**，
而不是悄悄搬走（派生节点按 type 路由，所以出现在 outbounds 里的只可能是模块自己写的，
报错能直接指出该改哪个文件）。

### 排序

`outbounds` 数组和每个 selector 的成员列表用**同一条规则**：

```
① 模块文件自带的条目      原位原序
② 中继节点                按 nodes.relays 的声明顺序
③ 普通节点                按 nodes.subscriptions 的声明顺序
```

②③用的都是**声明顺序**，与规则里 `relays` / `from` 的书写顺序无关。所以同一批节点在
任何 selector 里顺序都一致，调整某条规则的书写不会让产出重排。

同一条中继声明内部按「模板 × 上游」展开，模板在外层、上游在内层，两者都按节点池顺序
（= 订阅声明序 → 该订阅源文件内的出现序）。所以 `include: ["香港"]` 命中 3 个节点时，
产出的 3 个中继顺序就是这 3 个节点在机场订阅文件里的相对顺序，不排序。

---

## `configs`

每一项描述一个产出文件。

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `name` | string | ✅ | 产出名，唯一，用于日志和错误信息 |
| `path` | string | ✅ | 产出路径，解析规则见 `output` |
| `modules` | string[] | ✅ | 要组装的模块名，**按此顺序合并** |

校验：`name` 不能重复；`modules` 不能为空、每一项必须存在、不能重复列同一个模块。

---

## `update_schedule`

控制**定时重抓订阅**的周期。

```json
{ "type": "interval", "every": "6h" }
{ "type": "hourly" }
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|:---:|---|
| `type` | string | ✅ | `interval` 或 `hourly` |
| `every` | string | ⚠️ | `type=interval` 必填。Go duration：`"30m"` / `"6h"` / `"24h"` |

- `interval` —— 每隔 `every` 一次，从进程启动开始计时
- `hourly` —— 每个整点

**它和 GitHub 的配置变更触发是两件不同的事**：

- `update_schedule` 管「**订阅内容**可能变了」。机场节点会增删改，即使你的配置一个字
  没动，也需要定期重抓
- webhook / 兜底轮询管「**你的配置**变了」

两者独立，都会触发一次完整的产出流程。

---

# 三、产出校验

写盘之前全部在内存里完成，任何一条不过就不写任何文件——现有产出保持原样：

- 每个 selector / urltest 的成员非空
- 成员和 `detour` 引用的 tag 必须在同一文件内存在（悬挂引用）
- `outbounds` 与 `endpoints` 的 tag 全局唯一
- `outbounds` 里不含 endpoint 专属类型
- 每个数组段都是对象数组
- 产出不能是空对象

# 四、失败时的行为

| 情况 | 行为 |
|---|---|
| 拉取配置仓库失败 | 用最近一次拉到的快照继续跑，WARN 记录 |
| 快照解析 / 校验失败 | 结束，不写任何文件 |
| 部分订阅抓取失败 | WARN 记录，用成功的那些继续 |
| **全部**订阅抓取失败 | 结束，不写任何文件（否则会产出没有节点的配置） |
| 组装或产出校验失败 | 结束，不写任何文件，错误记入 `state.json` |
| 写盘中途失败 | 已成功的保留，失败的保持旧内容（每个文件独立原子） |
| 产出后发现不对 | `node-box rollback` 回到上一个被应用的快照重新产出 |
