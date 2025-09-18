# Node Box

一个用于管理和更新 SingBox 配置文件的工具，支持从 Clash 和 SingBox 订阅自动获取节点信息。

## 主要功能

- 支持 Clash 和 SingBox 订阅格式
- 自动转换 Clash 代理配置到 SingBox 格式
- 支持 HTTP/HTTPS/SOCKS5 代理获取订阅
- 支持自定义 User-Agent（全局和订阅级别）
- 智能重试机制和订阅缓存优化
- 定期自动更新功能

## 快速开始

### 安装

```bash
# 克隆项目
git clone <repository-url>
cd node-box

# 构建
go build -o bin/node-box ./cmd/node-box
```

### 使用

```bash
# 初始化配置文件
./bin/node-box init config.json

# 运行
./bin/node-box
```

## 文档

详细文档请查看 `doc/` 目录：

- [安装指南](doc/installation.md) - 详细的安装和构建说明
- [配置说明](doc/configuration.md) - 完整的配置文件格式和选项
- [使用指南](doc/usage.md) - 详细的使用方法和示例
- [开发文档](doc/development.md) - 项目架构和开发指南
- [API 文档](doc/api.md) - 包级别的 API 说明

## 许可证

GPL-3.0 license
