# 使用指南

## 基本使用

```bash
# 使用默认配置文件 config.json
./bin/node-box

# 使用自定义配置文件
./bin/node-box custom-config.json
```

## 初始化配置

```bash
# 生成默认配置文件
./bin/node-box init

# 生成到指定路径
./bin/node-box init /path/to/config.json
```

## 工作原理

1. 程序启动时读取配置文件
2. 根据配置的代理设置创建 HTTP 客户端
3. 通过代理或直连获取订阅内容
4. 解析订阅数据并转换为 SingBox 格式
5. 更新配置文件中的节点列表
6. 定期执行更新任务

## 支持的订阅类型

### Clash
- Shadowsocks
- VMess
- VLESS
- Trojan
- 支持 WebSocket 和 TLS 配置

### SingBox
- 保留所有原始字段
- 自动添加订阅前缀到节点标签

## 注意事项

- 确保配置文件中的 `insert_marker` 是一个 selector 类型的节点
- 代理配置是可选的，如果不配置则使用直连
- 程序会自动过滤包含排除关键词的节点
- 建议设置合理的更新间隔，避免过于频繁的请求
- 所有内部包都位于 `internal/` 目录下，不对外暴露