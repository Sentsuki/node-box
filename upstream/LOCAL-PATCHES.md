# upstream/ 的本地改动

这个目录是 [clash2singbox](https://github.com/xmdhs/clash2singbox) 的 vendored 副本，
**不参与 node-box 自身的重构和审计**。

但它不是原样照搬——下面两处改动是 node-box 需要的，
**每次从上游同步都会被覆盖，必须重新打一遍**。

升级完记得对照本文件核对一遍，然后跑 `go test ./upstream/...` 和 `go build ./...`。

---

## 1. import 路径改写

上游用的是它自己的 module path，vendored 进来之后要改成 `node-box/upstream/...`。

涉及 **53 行 / 33 个文件**，三种形态：

```
node-box/upstream/model
node-box/upstream/model/clash
node-box/upstream/model/singbox
```

同步后重新改写（把 `<上游原路径>` 换成上游实际的 module path）：

```bash
find upstream -name '*.go' -exec sed -i \
  -e 's#<上游原路径>/model/singbox#node-box/upstream/model/singbox#g' \
  -e 's#<上游原路径>/model/clash#node-box/upstream/model/clash#g' \
  -e 's#<上游原路径>/model#node-box/upstream/model#g' {} +
```

顺序有讲究：**长路径必须先替换**，否则 `/model` 会先把 `/model/clash` 的前缀吃掉。

验证没有漏网的：

```bash
grep -rn "<上游原路径>" upstream/ --include=*.go   # 应该为空
```

---

## 2. `alter_id` 去掉 `omitempty`

`upstream/model/singbox/singbox.go`：

```go
// 上游原样
AlterID  int  `json:"alter_id,omitempty"`

// node-box 需要的
AlterID  int  `json:"alter_id"`
```

**为什么**：Go 的 `omitempty` 认为 int 的 `0` 是空值，于是 `alter_id: 0` 会被整个丢掉。
产出的配置里应该如实写出订阅给的值，而不是靠 sing-box 的默认值兜着。

**副作用（已知并接受）**：`AlterID` 是 `SingBoxOut` 上所有类型共用的字段，去掉
`omitempty` 之后，clash 转出来的**每一个**节点都会带 `alter_id: 0`，不只 vmess：

```
vmess       {"alter_id":0, ...}
trojan      {"alter_id":0, ...}
shadowsocks {"alter_id":0, ...}
```

这是长期以来的行为，保持不变。

重新打补丁：

```bash
sed -i 's|`json:"alter_id,omitempty"`|`json:"alter_id"`|' \
  upstream/model/singbox/singbox.go
```

---

## 同步后的检查清单

```bash
grep -rn "<上游原路径>" upstream/ --include=*.go        # 空
grep -n "alter_id" upstream/model/singbox/singbox.go   # 不含 omitempty
go build ./...
go test ./upstream/... ./internal/subscription/...
```

另外留意 `internal/build/inject.go` 里的 `endpointTypes`：它必须覆盖
`upstream/convert/convert.go` 的 `typeMap` 能产出的全部 endpoint 类型
（当前是 `wireguard` 和 `openvpn-client`）。上游新增 endpoint 协议时要同步加进去，
否则那类节点会被错误地写进 `outbounds`，sing-box 启动会失败。
