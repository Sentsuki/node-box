# 配置文件参考手册

node-box 使用 JSON 格式的配置文件。可通过命令 `node-box init config.json` 生成示例配置。

---

## 顶层结构

```json
{
  "nodes":           { ... },   // 必填 - 节点订阅与全局过滤设置
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

节点配置负责定义订阅源以及全局适用的过滤规则。注意：这里不再定义目标文件及其代理节点写入规则，文件写入等操作已全部迁移至 [`modules.outbounds`](#modulesoutbounds--特殊字段与逻辑) 中配置。

### `nodes.subscriptions` — 订阅源列表

定义所有订阅来源，每个订阅为一个对象。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | ✅ | 订阅唯一名称，用于在 outbounds 的 subscriptions 中引用，并在节点 tag 中作为前缀标识 |
| `url` | string | ⚠️ | 远程订阅 URL，与 `path` 二选一 |
| `path` | string | ⚠️ | 本地订阅文件路径，与 `url` 二选一 |
| `type` | string | ✅ | 订阅格式：`"clash"` / `"singbox"` / `"relay"` |
| `enable` | bool | ✅ | 是否启用该订阅 |
| `remove_emoji` | bool | ❌ | 是否移除节点名称中的 Emoji 表情，默认 `false` |
| `user_agent` | string | ❌ | 自定义 User-Agent，优先级高于全局设置 |

**订阅类型说明：**

- **`clash`** — Clash YAML 格式的订阅，会自动转换为 sing-box 格式。支持常见协议（vmess, vless, trojan 等）。
- **`singbox`** — 原生 sing-box JSON 格式的订阅，直接提取 `outbounds` 中的代理节点（排除 `direct`、`block`、`selector` 等类型）。
- **`relay`** — 中继模板订阅。节点不会直接作为普通节点写入，而是与其他普通节点做笛卡尔积组合成连通性中继节点（详见 [Relay 处理机制](#relay-处理机制)）。

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
      "name": "入口备用",
      "url": "https://example.com/relay/sub",
      "type": "relay",
      "enable": true
    }
  ]
}
```

---

### `nodes.exclude_keywords` — 全局排除关键词

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `exclude_keywords` | string[] | ❌ | 全局排除关键词列表。tag 中包含任一关键词的节点会在提取解析阶段直接舍弃，不经过后续写入逻辑 |

**示例：**
```json
{
  "exclude_keywords": ["过期", "失效", "测试", "故障转移", "流量"]
}
```

---

### `nodes.relay_nodes` — Relay 节点全局写入规则

决定哪些展开后的 relay 节点组合会作为真实的 outbound 对象配置保留并写入最终文件。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `tag` | string | ✅ | relay 节点的 tag 中必须包含此关键词 |
| `upstream` | string[] | ✅ | relay 节点的 tag 中必须同时包含此列表中的至少一个关键词 |

**规则：**一个 relay 节点要被写入，必须同时满足：tag 包含 `tag` 关键词 **并且** tag 包含 `upstream` 数组中的任一关键词。若该列表为空或未配置，则相当于不写入任何 relay 组装节点。

**示例：**
```json
{
  "relay_nodes": [
    {
      "tag": "HK入口",
      "upstream": ["US-1", "SG-1", "JP-1"]
    }
  ]
}
```

---

## `modules` — 模块定义

定义 sing-box 配置文件的各个模块片段。模块可以从本地文件及远程 URL 加载，用于最终组装（基于 `configs`）。

```json
{
  "modules": {
    "log":          [ ... ],
    "dns":          [ ... ],
    "inbounds":     [ ... ],
    "outbounds":    [ ... ],
    "route":        [ ... ],
    "experimental": [ ... ]
  }
}
```

每种类型可以设置多个模块组，基础结构字段：

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | ✅ | 模块唯一的引用名称，用于 `configs` 装配组合 |
| `path` | string | ⚠️ | 本地 JSON 模块文件路径，与 `from_url` 二选一 |
| `from_url` | string | ⚠️ | 远程 JSON 模块 URL，与 `path` 二选一 |

### `modules.outbounds` — 特殊字段与逻辑

出站模块（outbounds）相比于其他模块，支持额外的真实节点写入和 selector 出站标签的自动更新功能：

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `subscriptions` | string[] | ❌ | 指定要把哪些订阅（Subscription）解析出节点，并写入当前模块对应的文件里。为空则匹配所有订阅的节点（排除了全局 exclude 后） |
| `selectors` | Selector[] | ❌ | 定义该模块文件内目标 Selector 的自动化过滤和挂载规则 |

**核心工作机制：** 

如果在出站模块配置中启用了 `path` (本地文件读写)，更新将会触发：
1. **老旧配置清理**：在更新文件前，程序会优先进行清理操作：提取并移除模块文件中之前生成和注入过的旧有订阅节点（节点及 selector tag 中带有 `[订阅名]` 前缀的项将被彻底剔除），防止因重命名订阅遗留的无法追踪的历史数据残存。
2. **专属作用域限制**：`subscriptions` 获取条件与 `selectors` 更新插入策略仅限应用于当前包含 `path` 所在的单个模块文件，不会波及同组的其他 outbounds 模块。这样便可轻易实现不同的订阅集群独立存放及互不干扰的任务调度。
3. **只读规则**：对于配置了 `from_url` (远程网络文件) 的 outbounds 模块，因为不具备回写条件而只能被视为静态只读模块，程序会直接跳过该阶段任何对其的节点注入和过滤处理。

### `modules.outbounds[].selectors` — Selector 更新规则

用于将满足过滤匹配规则节点名 (tag)，追加至指定名称 selector （出站选路）的 `outbounds` 列表下：

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `insert_marker` | string | ✅ | 定位标记，须完全匹配要处理的目标 selector 的 `tag` 名称 |
| `include_nodes` | string[] | ❌ | 包含词筛选。真实节点 tag 包含这几个关键字中任意一个及以上的，都会被推入此 selector |
| `exclude_nodes` | string[] | ❌ | 排除词筛选。真实节点 tag 中存在列表里字符任意一项便会从对应 selector 中刷掉（优先级通常高于 include_nodes） |
| `include_relay_nodes` | string[] | ❌ | 针对 Relay 类型的专门筛选列表。决定哪一部分已合成的 relay 相关节点 tag 会进入并被此 selector 使用 |

**示例：**

```json
{
  "modules": {
    "outbounds": [
      {
        "name": "main_outbound",
        "path": "./modules/outbounds/main.json",
        "subscriptions": ["主力订阅", "备用节点"],
        "selectors": [
          {
            "insert_marker": "🚀 节点选择",
            "include_nodes": ["香港", "新加坡", "美国"],
            "exclude_nodes": ["故障", "内部测试"],
            "include_relay_nodes": ["HK入口"]
          },
          {
            "insert_marker": "🎬 流媒体",
            "include_nodes": ["Netflix", "Disney"]
          }
        ]
      }
    ]
  }
}
```

---

## Relay 处理机制

Relay 是 node-box 的高级功能设计，用于灵活构建链式代理网与流量接力机制（多跳转发）。由于 relay 各个子规则的作用范围独立，整体处理流程按先后经过四步：

### 1. 模板展开组装
`type: "relay"` 在订阅配置内的判定是被视为了中继特征模板。程序会把此类下属所有成员节点同所有已被正常保留的普通节点进行自动化的笛卡尔积组装：
- 组装后的新节点 `detour`（直连脱节向指代值）会被赋上上游被穿透对象普通节点的 tag 属性名。
- 新的合并后节点本身的 `tag` 名称构成规律为：`[relay订阅名] 源模板节点原始名称 普通节点tag名称`。

### 2. `nodes.relay_nodes` 全局初步保留
全部经过上阶段组合出来的海量散装 relay 候选体，将借由最外侧配置内 `nodes.relay_nodes` 配置逻辑作为第一道分流门槛，只截留过滤满足了（本身携带了约束 `tag` 和穿层对应 `upstream` 相关名）的合规项，剩余的一律被清退销毁不会向任何下游流转。

### 3. `modules.outbounds[].subscriptions` 模块归类落地
对于合规留存经过第二步检验的 relay 中继节点集，当匹配到达具体的 Outbounds 输出配置阶段，再根据此时其隶属 `outbounds` 配置给定的 `subscriptions` 是否接纳并涵盖该部分订阅（通过检索节点特征名上的带有被方括号标记出来的订阅名作为特征条件比对）。如果被圈中，才会真正随同原始正常结构化模型实体一起作为 outbounds 配置块保存落盘至当前对应的实体文件之中。

### 4. `include_relay_nodes` 面向 Selector 的按需使用
最后在针对 `selectors` 对已写入该文件的节点进行标签取用的业务里，借助特定配置好 `include_relay_nodes` 检索关键字匹配那些确有需要挂接到选择器策略下执行后续实务访问分发的上述指定 relay 脱胎类节点 tag 名称并完成最终更新推入动作。

---

## `configs` — 配置文件合成组装

定义如何将分布各处的各个片段模块组装拼合成完整用于 Sing-Box 核心加载运转的单一聚合式总配置文件。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | ✅ | 组装策略名 |
| `path` | string | ✅ | 生成合并拼接后最终完整总配置输出及覆盖的目标物理文件归宿位置 |
| `modules` | string[] | ✅ | 要装配提取融合的所有预定义的各层级模块 `name` 集合清单 |
| `no_need_nodes` | string[] | ❌ | 针对末端产出执行的再筛净化列表。合成整合完毕时刻，倘使检查到 outbounds 或者 endpoints 项目里面含有 tag 匹配到以下字符的情况，会被彻底作为不需要的内容剔除抽离且清空连带从属关系 |

**合成附带自动处理特点：**
- 核心转换过程会自动感知 `wireguard` / `tailscale` 这一类型的出站节点，并做平移提取将其转存在 `endpoints` 顶层节点树逻辑架构区域。
- 自动化扫描 `endpoints` 内部那些携有方括号 `[]` 前缀源自订阅解析生成的残次无用节点碎片一并实施清理。
- 同步执行模块剥离过滤，自动抹除整合产物里部分冗余且没有任何使用指引填充内容的完全空集合。

**示例：**

```json
{
  "configs": [
    {
      "name": "main_config",
      "path": "./singbox/config.json",
      "modules": [
        "log_module",
        "dns_module",
        "main_outbound",
        "route_china",
        "route_proxy"
      ],
      "no_need_nodes": ["测试", "备用网络", "过期失效"]
    }
  ]
}
```

---

## `update_schedule` — 更新调度策略

控制 node-box 何时、以怎样的高可用性轮转周期去唤起拉取重做节点列表与刷新模块配置的任务。

| 字段 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `type` | string | ✅ | 可供应用的调度类型模式： `"interval"` 或 `"hourly"` |
| `interval` | int | ⚠️ | 执行时间隔休眠的时长单位定义（小时）。只有上述 `type` 定义值为 `"interval"` 时此项需进行设定 |

**模式说明与说明：**
- **`"interval"` 模式**：任务将会在节点服务挂载后的冷启动第一顺位刻度立马去跑过一遍大盘数据抽取与落地。其后依照该设置下每经过 N 小时后唤醒执行自循环轮转。
- **`"hourly"` 模式**：同样在启动伊始必定完整过一次执行队列，此后便固定向系统整点钟对齐，逢每一小时钟头切边必定准点重做。

**注意**：无论上述通过调度体系执行还是停止任务，服务在跑于后台生命周期的任意时刻下都会主动监视系统加载用的 config 配置文件的自我变化（特征码 MD5 哈希特征或修改存取状态时延比对）。一旦察觉手工/外部触发更改，总会不依赖任何调度的触发而抢先强插强制热重载全局刷新并实施所有的提取跟组装命令。

---

## 附加环境及其它

### `log_level` 日志输出等级
设定程序后台巡检运转时期的打印披露信息的程度。
可用枚举：`"silent"`, `"error"`, `"warn"`, `"info"`(程序默认倾向), `"debug"`。
此项支持被同名由系统环境变量中拉出的 `LOG_LEVEL` 进行初始影响覆盖定义，但配置脚本有最终更上的仲裁级别。

### `proxy` 探路代理
有些订阅拉取服务器本身阻隔未认证地区的互联，配置后包括外层云端组件获取及订阅网络访问所有的发起均利用此处的指定 HTTP 请求打通桥梁穿透通信（非填入此项时等式直连等效不应用代理）。
```json
{
  "type": "http", // 支持类型包含 http, https, socks5 规约体系
  "host": "127.0.0.1",
  "port": 7890
}
```

### `user_agent` 身份辨伪欺骗
主要负责进行对外资源网络通讯模拟客户端身份。
由于各个云订阅商端有自我设定的提取隔离机制限定，伪装识别层级按照从下向上传递规则覆盖决胜：
`订阅节点下自定义的 user_agent` > `根下赋予全局的 user_agent`  >  程序底线硬编码 `"sing-box"`。

### 核心支持环境变量

在没有指定配置或用于外层容器化脚本驱动调度时生效的基本环境变量群：

| 变量名 | 实际控制业务说明 |
|--------|------|
| `NODE_BOX_CONFIG` | 主配核心引用位置引导指针（其启动地位从低往上排序：默认配置常熟位置 < `NODE_BOX_CONFIG` 环境变量 < 用户执行命令行传递显示入参） |
| `LOG_LEVEL` | 常规未配置情况日志输出定死级 |
| `LOG_TIME` | 若设 `true` 或者 `1`，日志会强控启用携带时刻日历版带出的系统时刻标记码 |
| `LOG_PREFIX` | 自定义基础输出在打印时的字符串前缀占位提示 |
