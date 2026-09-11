# 部署

> 面向全新安装。环境假设：单台 VPS，有公网 IP / 域名，单一文件系统，root 权限。

## 1. 命令行

```
node-box run                          常驻：定时 + webhook + 兜底轮询
node-box update [--ref <sha>] [--force]  执行一次完整更新后退出
node-box pull [--ref <sha>]           只刷新快照，不产出
node-box build [--diff] [--ref <sha>] 只组装不写盘；--diff 打印与现有产出的差异
node-box validate [--ref <sha>]       校验快照能否组装出合法配置
node-box rollback                     切回 last-good 快照并重新产出
node-box status [--json]              当前 ref / 上次产出 / 上次错误
node-box init [目录] [--force]        生成配置仓库骨架
```

全局参数（每个子命令都接受）：

| 参数 | 说明 |
|---|---|
| `--config <path>` | 引导配置路径，优先级高于 `NODE_BOX_CONFIG` |
| `--log-level <lvl>` | 覆盖配置里的 `log_level` |

引导配置路径解析顺序：`--config` > `NODE_BOX_CONFIG` > 二进制同目录的 `node-box.json`。
显式给出的 `--config` 永远优先，不存在「显式路径输给环境变量」的情况。

`build --dry-run --diff` 是改配置时最有用的一条：它把产出算出来但不写盘，直接打印和当前
文件的差异。这在旧版本里做不到，因为组装过程和磁盘写入是缠在一起的。

---

## 2. 目录准备

```bash
install -d -m 0755 /opt/node-box
install -d -m 0700 /opt/node-box/snapshots
install -d -m 0700 /opt/node-box/state
install -d -m 0700 /opt/node-box/out

# 二进制
curl -fsSL -o /opt/node-box/node-box \
  https://github.com/you/node-box/releases/latest/download/node-box-linux-amd64
chmod 0755 /opt/node-box/node-box
```

`snapshots/` 里会有订阅 URL 的明文副本（token 在里面），所以是 `0700`。

### 引导配置

`/opt/node-box/node-box.json`：

```json
{
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

### 密钥

```bash
umask 077
cat > /opt/node-box/.env <<EOF
NODE_BOX_GH_TOKEN=github_pat_xxxxxxxxxxxxxxxx
NODE_BOX_WEBHOOK_SECRET=$(openssl rand -hex 32)
EOF
chmod 0600 /opt/node-box/.env
```

把生成的 `NODE_BOX_WEBHOOK_SECRET` 记下来，下一步要填进 GitHub。

GitHub token 用 **fine-grained PAT**，Repository access 只勾配置仓库，权限只需要
**Contents: Read-only**。

---

## 3. 配置仓库

在 GitHub 上建一个 **private** 仓库，例如 `you/node-box-config`：

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

`node-box init` 可以生成这个骨架。字段含义见 `configuration.md`。

### 仓库 Secret

仓库 → Settings → Secrets and variables → Actions → New repository secret：

| 名称 | 值 |
|---|---|
| `NODEBOX_WEBHOOK_SECRET` | 与 `/opt/node-box/.env` 里的 `NODE_BOX_WEBHOOK_SECRET` **完全一致** |
| `NODEBOX_URL` | `https://nb.example.com/hooks/github` |

### Action

`.github/workflows/notify.yml`：

```yaml
name: Notify node-box

on:
  push:
    branches: [main]
    paths:
      - 'config.json'
      - 'modules/**'
  workflow_dispatch:

concurrency:
  group: notify-node-box
  cancel-in-progress: true

jobs:
  notify:
    runs-on: ubuntu-latest
    steps:
      - name: Ping node-box
        env:
          SECRET: ${{ secrets.NODEBOX_WEBHOOK_SECRET }}
          URL: ${{ secrets.NODEBOX_URL }}
        run: |
          BODY=$(jq -nc --arg ref "$GITHUB_SHA" '{ref:$ref}')
          SIG=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -r | cut -d' ' -f1)
          curl -sS --fail-with-body --max-time 20 \
            -H 'Content-Type: application/json' \
            -H "X-NodeBox-Signature-256: sha256=$SIG" \
            -d "$BODY" \
            "$URL"
```

几个要点：

- `paths:` 过滤 —— 改 README 不会触发更新
- `workflow_dispatch` —— 给你一个手动触发的按钮
- `concurrency` —— 连续 push 时只保留最后一次
- `--fail-with-body` —— node-box 返回非 2xx 时 Action 标红，错误信息直接显示在 Action 日志里，
  不用登录 VPS 翻日志

---

## 4. 反向代理

node-box 只监听 `127.0.0.1:8788`，TLS 和公网入口交给 Caddy。

`/etc/caddy/Caddyfile`：

```caddyfile
nb.example.com {
    handle /hooks/github {
        reverse_proxy 127.0.0.1:8788
    }
    handle /healthz {
        reverse_proxy 127.0.0.1:8788
    }
    respond 404
}
```

`/status` 没有暴露到公网——它会回显配置状态，本机 `curl` 看就行。

不要让 node-box 自己管证书：少一大块攻击面和运维面。

---

## 5. systemd

`/etc/systemd/system/node-box.service`：

```ini
[Unit]
Description=node-box
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/node-box
ExecStart=/opt/node-box/node-box run --config /opt/node-box/node-box.json
EnvironmentFile=/opt/node-box/.env
Restart=on-failure
RestartSec=10s
UMask=0077

# 收敛权限
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths=/opt/node-box /etc/sing-box

[Install]
WantedBy=multi-user.target
```

`ReadWritePaths` 要把所有产出目标目录列进去。如果 `configs[].path` 只用 `output.dir` 下的
相对路径，那 `/opt/node-box` 一个就够。

```bash
systemctl daemon-reload
systemctl enable --now node-box
journalctl -u node-box -f
```

### 优雅停止

node-box 监听 `SIGINT` / `SIGTERM`：收到后取消 context，等当前这一轮更新做完再退出。
因为写盘是原子的，即使在写入过程中被强杀，产出文件也只会是完整的旧版或完整的新版。

`SIGHUP` 触发一次立即更新，等价于 `node-box update`，但不需要新起进程：

```bash
systemctl kill -s HUP node-box
```

---

## 6. 首次运行

先不要直接启动服务，手动跑一遍确认链路通：

```bash
cd /opt/node-box
set -a; . ./.env; set +a

./node-box pull                  # 能不能拉到仓库
./node-box validate              # 配置能不能组装
./node-box build --dry-run --diff  # 看看会产出什么
./node-box update                # 真的写一次
```

确认 `out/` 下的文件符合预期后，再 `systemctl enable --now node-box`。

验证 webhook 链路：在 GitHub 仓库页面 Actions → Notify node-box → Run workflow，
点一下，看 Action 是否绿、`journalctl` 里有没有对应的日志。

---

## 7. 日常操作

**改配置**：在 GitHub 上编辑 → commit → Action 自动 ping → 几秒后产出更新 → 手动重启 sing-box。

**看状态**：

```bash
curl -s 127.0.0.1:8788/status | jq
./node-box status
```

**产出有问题**：

```bash
./node-box rollback     # 切回 last-good 并重新产出
```

**临时停掉自动更新**：

```bash
systemctl stop node-box
```

产出文件不会被动，sing-box 继续用现有配置跑。

---

## 8. 备份与迁移

整个状态就是一个目录：

```bash
tar -czf node-box-backup.tar.gz -C /opt node-box
```

迁移到新机器：拷过去、改 `node-box.json` 里的路径（如果变了）、重装 systemd unit。
快照和 state 都能直接复用。

真要从零恢复也很快——`snapshots/` 是缓存，删掉之后下一次运行会重新从 GitHub 拉。
**唯一不可再生的是 `.env` 里的两个密钥**，单独记在密码管理器里。

---
