# 架构

> 本文描述 node-box 重构后的架构。outbounds / endpoints 两个块的处理逻辑尚未定稿，
> 文中相关位置标注为 **【待定】**。

## 1. 设计原则

1. **GitHub 是唯一真相源。** 本地只存两种东西：不可变快照、可再生产物。没有第三种状态。
2. **组装是纯函数。** `Build(Snapshot, Nodes) → []OutputFile` 不碰磁盘、不读全局状态。
3. **所有触发走同一条串行管道。** 定时、webhook、手动、信号，进同一个 channel。

原则 1 和 2 合起来意味着：**模块文件永远不会被写入**。配置的产出是每次从零重建的，
因此不需要"识别并清理上一轮自己写进去的内容"这类补偿逻辑。

## 2. 目录布局

一切收在 `/opt/node-box/` 下，自包含。备份就是 `tar` 整个目录，迁移就是拷贝。

```
/opt/node-box/
├── node-box                    # 二进制
├── node-box.json               # 本地引导配置（见 configuration.md）
├── .env                        # 0600，GitHub token 与 webhook secret
├── snapshots/
│   ├── a1b2c3d.../             # 不可变快照，按 commit sha 命名
│   ├── d4e5f6a.../
│   ├── current    -> d4e5f6a...   # 最近一次成功拉取
│   └── last-good  -> a1b2c3d...   # 最近一次成功产出
├── state/
│   └── state.json              # 上次 ref、各产出文件的 hash、上次错误
├── out/                        # 默认输出目录，可被 output.dir 覆盖
│   ├── main.json
│   └── gaming.json
└── logs/                       # 可选；默认直接走 stdout / journald
```

`current` 与 `last-good` 是 `snapshots/` 内部的相对软链，所以整个目录可以整体移动。

快照保留最近 10 个，超出的按时间从旧到新删除；`current` 和 `last-good` 指向的永不删除。

## 3. 运行流程

一次完整运行（不论由什么触发）都走同一条路径：

```
① 触发          Trigger{Kind, Ref, Force} 进入 channel，runner 串行消费
       ↓
② 拿快照        GET /repos/{owner}/{repo}/tarball/{ref}，带 If-None-Match
                304 → 复用 current           200 → 解压到 snapshots/<sha>/，切 current
       ↓
③ 读配置        从快照读 config.json → 解析 → Validate
                同时把 modules/ 下被引用的文件读进内存
                ※ 从这一刻起，snapshots/ 目录只读，不再被碰
       ↓
④ 抓订阅        并发抓取，带 ctx 与响应体大小上限，解析成 map[订阅名][]Node
       ↓
⑤ 组装          Build(snapshot, nodes) —— 全内存，零磁盘 IO
                合并模块 → 注入节点【待定】→ 序列化
       ↓
⑥ 校验          在内存里检查产出合法性，不合法就地失败，不写盘
       ↓
⑦ 写盘          逐文件比对 hash：未变则跳过；已变则 tmp → fsync → rename
       ↓
⑧ 收尾          更新 state.json，last-good 指向本次 sha
```

### ⑤ 组装的内部顺序

```
doc := {}
for each module in configs[].modules:
    把模块 JSON 的顶层键合并进 doc      # 键冲突 → 报错，见 configuration.md
【待定】outbounds / endpoints 处理
删除所有值为 null / [] / {} 的顶层键
json.MarshalIndent(doc, "", "  ") + 末尾换行
```

产出是确定性的：Go 的 `encoding/json` 对 map 键按字典序排序，数组保序。

## 4. 容错规则

失败不应该导致坏配置落盘。每一类失败都有明确行为：

| 情况 | 行为 |
|---|---|
| 拉取 GitHub 失败 | 用 `current` 快照继续跑，WARN 记录，**不静默** |
| 快照解析/校验失败 | 结束，`current` 不前进，不写任何文件 |
| 部分订阅抓取失败 | WARN 记录，用成功的那些继续 |
| **全部**订阅抓取失败 | 结束，不写任何文件，现有产出原封不动 |
| 组装失败 | 结束，不写任何文件 |
| 产出校验失败 | 结束，不写任何文件，错误写入 `state.json` |
| 写盘中途失败 | 已成功的文件保留，失败的保持旧内容（每个文件独立原子） |
| 产出后发现有问题 | `node-box rollback` 切回 `last-good` 重新产出 |

关键点：**任何失败路径都不会产生半成品文件**。

## 5. 触发模型

```go
type Trigger struct {
    Kind  Kind   // Startup | Schedule | Poll | Webhook | Signal | Manual
    Ref   string // 期望的 commit sha；空表示取分支最新
    Force bool   // 跳过 hash 短路，强制重写
}
```

单个 runner goroutine 串行消费一个 buffered channel：

```go
for {
    select {
    case <-ctx.Done():
        return
    case t := <-triggers:
        t = coalesce(t, triggers)   // 排空积压并合并，天然防抖
        runOnce(ctx, t)
    }
}
```

串行消费天然解决并发更新、重复触发、webhook 风暴，不需要额外加锁。

| 触发源 | 说明 |
|---|---|
| `Startup` | 进程启动后立即执行一次 |
| `Schedule` | 按 `update_schedule` 定时 |
| `Poll` | 兜底轮询发现 ETag 变化 |
| `Webhook` | 收到 GitHub Action 的通知 |
| `Signal` | 收到 `SIGHUP` |
| `Manual` | `node-box update` 子命令（一次性进程，不走常驻 runner） |

## 6. 两个时钟

这是容易混淆的地方：**定时更新和 GitHub 触发管的是两件不同的事。**

- **`update_schedule`** 管的是「**订阅内容**可能变了」。机场的节点会增删改，即使你的配置一个字没动，也需要定期重新抓订阅并重新产出。
- **webhook / 兜底轮询** 管的是「**你的配置**变了」。你在 GitHub 上改了 `modules/route.json`，希望立刻生效。

两者互相独立，都会触发一次完整的 ⑤~⑧ 流程。

## 7. webhook

node-box 内置一个最小 HTTP server，**只监听 `127.0.0.1`**，TLS 由前置的 Caddy / nginx 负责。

```
POST /hooks/github     唯一的写入口
GET  /healthz          存活探测
GET  /status           当前 ref / 上次产出时间 / 上次错误
```

请求格式：

```http
POST /hooks/github HTTP/1.1
Content-Type: application/json
X-NodeBox-Signature-256: sha256=<hex>

{"ref":"d4e5f6a..."}
```

签名是对**原始 body 字节**做 HMAC-SHA256，密钥来自 `.env` 里的 `NODE_BOX_WEBHOOK_SECRET`，
用 `hmac.Equal` 做常数时间比较。

| 响应 | 含义 |
|---|---|
| `202 Accepted` | 已入队。**立刻返回，更新在后台跑** |
| `400 Bad Request` | body 不合法 |
| `401 Unauthorized` | 签名校验失败 |
| `429 Too Many Requests` | 触发限流 |

其他约束：

- body 上限 64 KB
- `http.Server` 显式设置 `ReadHeaderTimeout` / `ReadTimeout` / `WriteTimeout`（Go 默认全是 0）
- 只接受 POST，路径精确匹配
- 每 IP 限流 + 全局令牌桶

### 为什么必须保留兜底轮询

webhook 会丢：node-box 重启窗口、网络抖动、Action 排队超时。默认每 10 分钟带 `If-None-Match`
轮询一次做对账。GitHub 对返回 304 的条件请求不计入 rate limit，所以这个成本接近于零。

**只靠 webhook 是不可靠的。**

## 8. 为什么模块用 `file` 而不是 `from_url` 指向 raw

1. **原子一致性** —— tarball 是单个 commit 的完整快照。逐个 URL 拉取可能横跨你的两次 push，
   组装出一个从未存在过的配置组合。
2. **绕开 raw CDN 缓存** —— `raw.githubusercontent.com` 有最长约 5 分钟缓存。Action 触发后
   node-box 可能拉到旧内容，"立即更新"静默失效。
3. **N+1 次请求变 1 次** —— 少 N 个失败点。
4. **private 仓库鉴权干净** —— tarball 走标准 API，一个 PAT 搞定。
5. **"版本"这个概念才成立** —— 快照按 sha 存储，`rollback` / `--ref` / `status` 才有意义。
6. **离线可用** —— 拉取失败时 `current` 快照里模块内容是全的。

`from_url` 保留，但用途收窄为引用**别人维护的**第三方模块。

## 9. 包结构

```
cmd/node-box/          main / 子命令 / 信号处理
internal/
  model/               配置结构体 + Validate
  source/
    source.go          type Source interface { Fetch(ctx, ref) (*Snapshot, error) }
    github.go          tarball + ETag + PAT
    local.go           本地目录实现（开发 / 离线）
    snapshot.go        Snapshot 类型
    store.go           快照落盘 / current / last-good / GC
  fetch/               HTTP client：ctx、并发、响应体上限、重试
  subscription/        订阅解析 → []Node（含上游 clash/convert、clash/model）
  build/
    build.go           Build(Snapshot, Nodes) ([]output.File, error)
    assemble.go        模块合并
    nodes.go           【待定】outbounds / endpoints 注入
    validate.go        产出校验
  output/              File 类型、hash 短路、原子写
  runner/              Trigger 管道 + 调度 + 一次性执行
  webhook/             HTTP server
  logger/
```

## 10. 原子写

所有产出文件都通过同一个函数落盘：

```go
func WriteAtomic(path string, data []byte, perm os.FileMode) error
```

实现：在**目标文件所在目录**创建 `.<name>.*.tmp` → 写入 → `Chmod` → `Sync` → `Rename`。

`rename(2)` 保证其他进程看到的要么是完整旧内容、要么是完整新内容，不存在中间态。
tmp 文件必须和目标同目录（而不是统一放在某个 tmp 目录），因为 `rename` 跨文件系统会返回 `EXDEV`。

产出文件权限 `0600`。

## 11. 【待定】outbounds / endpoints

以下逻辑尚未定稿，当前阶段不实现：

- 订阅节点如何注入 outbounds
- selector / urltest 的成员如何计算（现有的 `include_nodes` / `exclude_nodes` 语义需要重新设计）
- relay 节点的展开规则（现有的"全笛卡尔积再过滤"在节点多时会爆）
- `wireguard` / `tailscale` 类型如何归入 endpoints
- `no_need_nodes` 这类产出级过滤的位置

在 `build` 包中，这部分通过一个明确的接口留作插入点：

```go
// nodes.go
type NodeInjector interface {
    Inject(doc map[string]any, cf *model.ConfigFile, nodes map[string][]Node) error
}
```

在该接口定稿前，实现为 passthrough：模块里写了什么 outbounds / endpoints，产出里就是什么。

定稿后需要补充的产出校验：

- 每个 selector / urltest 的 `outbounds` 非空（不能是 `null` 或 `[]`）
- selector 成员引用的 tag 在同一文件内真实存在（悬挂引用检查）
- 全局 tag 唯一
- 顶层 `outbounds` 非空
