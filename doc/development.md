# 开发文档

## 代码结构

项目采用分层架构设计：

1. **cmd/node-box**: 程序入口，处理命令行参数
2. **internal/config**: 配置管理层
3. **internal/client**: 网络请求层
4. **internal/subscription**: 数据处理层
5. **internal/fileops**: 文件操作层
6. **internal/manager**: 业务逻辑层

## 添加新的订阅类型

1. 在 `internal/subscription` 包中实现 `Processor` 接口
2. 在 `NewProcessor` 函数中注册新的处理器
3. 添加相应的测试用例

## 测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./internal/config

# 运行测试并显示覆盖率
go test -cover ./...
```

## 代码质量检查

```bash
# 格式化代码
go fmt ./...

# 静态分析
go vet ./...

# 使用 golint（需要安装）
golint ./...
```

## 贡献

1. Fork 本项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request