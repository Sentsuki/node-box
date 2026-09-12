# upstream/ 本地改动

该目录为 [clash2singbox](https://github.com/xmdhs/clash2singbox) 的 vendored 副本。从上游同步后需重新应用以下改动：

## 1. 改写 import 路径

将上游 module path 替换为 `node-box/upstream/...`：

```bash
find upstream -name '*.go' -exec sed -i \
  -e 's#github.com/xmdhs/clash2singbox/model/singbox#node-box/upstream/model/singbox#g' \
  -e 's#github.com/xmdhs/clash2singbox/model/clash#node-box/upstream/model/clash#g' \
  -e 's#github.com/xmdhs/clash2singbox/model#node-box/upstream/model#g' {} +
```

## 2. `alter_id` 移除 `omitempty`

文件：`upstream/model/singbox/singbox.go`

```diff
- AlterID  int  `json:"alter_id,omitempty"`
+ AlterID  int  `json:"alter_id"`
```

## 3. 验证与注意事项

```bash
grep -rn "github.com/xmdhs/clash2singbox" upstream/ --include=*.go  # 确认无残留引用
grep -n "alter_id" upstream/model/singbox/singbox.go                # 确认不含 omitempty
go test ./upstream/... ./internal/subscription/...
go build ./...
```

> [!NOTE]
> 检查 `internal/build/inject.go` 中的 `endpointTypes` 是否覆盖 `upstream/convert/convert.go`（`typeMap`）输出的全部 endpoint 类型（当前为 `wireguard` 和 `openvpn-client`）。若上游新增 endpoint 协议需在此同步补充。
