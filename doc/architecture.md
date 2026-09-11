# 架构

> 本文描述 node-box 重构后的架构。

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
│   ├── current                 # 文本文件，内容是最近一次成功拉取的 ref
│   └── last-good               # 文本文件，内容是最近一次成功产出的 ref
├── state/
│   └── state.json              # 上次 ref、各产出文件的 hash、上次错误
├── out/                        # 默认输出目录，可被 output.dir 覆盖
│   ├── main.json
│   └── gaming.json
└── logs/                       # 可选；默认直接走 stdout / journald
```

`current` 与 `last-good` 是**纯文本指针文件**而不是软链：指针文件用和别处一样的
`tmp + rename` 原子替换，不需要创建软链的权限，各平台行为一致，整个目录也可以整体移动。
指向一个已被删除的快照时读作「未设置」，而不是交回一个打不开的 ref。

快照保留最近 10 个，超出的按时间从旧到新删除；`current` 和 `last-good` 指向的永不删除。
被中断的拉取会在 `snapshots/.incoming-*` 留下临时目录，进程启动时清理。

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
                合并模块 → 注入节点 → 序列化
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

needed := {}
for each selector 规则（来自本产出引用的模块）:
    members := 解析规则                 # from/include/exclude + relays
    doc 里那个 selector 的 outbounds = 自带成员 + members
    needed |= members
needed |= 每个 needed 节点的 detour 目标   # 传递闭包
把 needed 写入 doc                        # wireguard/tailscale → endpoints

删除所有值为 null / [] / {} 的顶层键
json.MarshalIndent(doc, "", "  ") + 末尾换行
```

### 排序规则

`outbounds` 数组和每个 selector 的成员列表用**同一条规则**：

```
① 模块文件自带的条目        原位原序
② Relay 链式节点            按 nodes.relays 的声明顺序
③ 普通节点                  按 nodes.subscriptions 的声明顺序
```

②③用的都是**声明顺序**，与规则里 `relays` / `from` 的书写顺序无关。所以同一批
节点在任何 selector 里顺序都一致，调整某条规则的书写也不会让产出重排。

同一条 relay 声明内部按「模板 × 上游」展开，两者都按节点池顺序。

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
cmd/node-box/
  main.go              子命令分发、全局参数、信号
  commands.go          run / update / pull / build / validate / rollback / status
  diff.go              build --diff 的行级 diff
  init.go              配置仓库骨架
internal/
  logx/                分级日志（level 用 atomic，webhook 与 runner 并发写）
  model/
    bootstrap.go       node-box.json
    config.go          仓库 config.json + 校验
    paths.go           产出路径解析
    duration.go        duration 字符串
  fetch/
    client.go          ctx / 代理 / 响应体上限 / ETag
    retry.go           只重试真正瞬时的失败
  textutil/            emoji 感知的字符串匹配
  subscription/
    node.go            Node 类型、深拷贝
    processor.go       clash / singbox / xray 解析
    fetcher.go         并发抓取 + 命名规则 + 全局 exclude
    transform.go       emoji / remove_keywords / 前缀
    xray/              分享链接解析
  source/
    snapshot.go        Snapshot、Source 接口、Acquire、模块加载
    store.go           快照落盘 / current / last-good / GC
    github.go          tarball + ETag + PAT + 安全解压
    local.go           本地目录（开发 / 离线）
  build/
    build.go           Build(Input) ([]output.File, error) —— 纯函数
    assemble.go        模块合并 + 键冲突检测
    inject.go          节点池、中继生成、NodeSelector 解析
    apply.go           selector 成员写入、detour 闭包、节点插入
    validate.go        产出校验
  output/
    file.go            File + 内容 hash
    write.go           原子写、hash 短路、目标目录校验
    state.go           state.json
  runner/
    runner.go          Trigger 管道 + BuildPlan + Execute
    loops.go           定时与兜底轮询
    trigger.go         Trigger 与合并
    status.go          Status / Rollback / Validate
  webhook/
    server.go          HMAC 校验、优雅关闭
    limiter.go         令牌桶
upstream/              第三方 clash2singbox，只改过 import 路径
```

`build` 不导入 `source`：它接受自己定义的 `build.Input`（配置 + 模块原始字节 +
节点 + 已解析的产出路径），返回内存里的文件。这样「配置从哪来」和「怎么组装」
彻底解耦，组装可以脱离网络和磁盘单测。

## 10. 原子写

所有产出文件都通过同一个函数落盘：

```go
func WriteAtomic(path string, data []byte, perm os.FileMode) error
```

实现：在**目标文件所在目录**创建 `.<name>.*.tmp` → 写入 → `Chmod` → `Sync` → `Rename`。

`rename(2)` 保证其他进程看到的要么是完整旧内容、要么是完整新内容，不存在中间态。
tmp 文件必须和目标同目录（而不是统一放在某个 tmp 目录），因为 `rename` 跨文件系统会返回 `EXDEV`。

产出文件权限 `0600`。

## 11. outbounds / endpoints

**selector 引用的节点是唯一真相，插入由引用派生。** 详见 `configuration.md` 3.4 和 3.6。

这条反转消掉了旧实现里一整套机制：

| 旧机制 | 为什么不再需要 |
|---|---|
| `configs[].no_need_nodes` | 没有多余节点可删 |
| `modules[].subscriptions` | 每条 selector 规则自己声明来源 |
| `include_relay_nodes` | 并入 `relays`，按名引用 |
| `nodes.relay_nodes` | 变成 `nodes.relays` 声明式定义 |
| 订阅 `type: "relay"` | 模板身份来自被 `via` 引用，不来自声明 |
| 「tag 含方括号」= 我生成的 | 全量重建，没有残留概念 |
| 中继全笛卡尔积再过滤 | 生成在声明时就有界 |

产出校验（写盘前全在内存里完成）：

- 每个 selector / urltest 的成员非空 —— 旧实现会写出 `"outbounds": null`
- 成员和 `detour` 引用的 tag 必须在同一文件内存在 —— 悬挂引用
- `outbounds` 与 `endpoints` 的 tag 全局唯一
- 每个数组段都是对象数组
