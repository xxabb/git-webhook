# conf.d/ 配置目录

Deploy Webhook Handler 的所有配置文件存放在本目录。

## 文件说明

### 应用配置

| 文件 | 说明 |
|------|------|
| `webhookd.example.yaml` | 应用配置示例文件 |

### Taskfile 示例

| 文件 | 对应应用 | 说明 |
|------|----------|------|
| `deploy_example.yaml` | 示例 | 通用 Taskfile 模板 |
| `deploy_saas.api.example.yaml` | `saas.api` | Go API 项目构建脚本 |
| `deploy_saas.web.example.yaml` | `saas.web` | Node.js 前端构建脚本 |
| `deploy_saas.grpc.example.yaml` | `saas.grpc` | Go gRPC 项目构建脚本 |

### 服务配置

| 文件 | 说明 |
|------|------|
| `webhook-handler.service` | systemd 服务文件 |

### 文档

| 文件 | 说明 |
|------|------|
| `README.md` | 本文件 |
| `TASKFILE-README.md` | Taskfile 使用说明和示例 |

## 协同机制

### webhookd.yaml 与 Taskfile 的关系

```
webhookd.yaml (webhookd.example.yaml)    deploy_saas.api.yaml (deploy_saas.api.example.yaml)
┌──────────────────────────────┐     ┌─────────────────────────────────┐
│ name: saas.api               │     │ tasks:                          │
│ taskfile: deploy_saas.api    │ ──> │   build:                        │
│ tasks:                       │     │     cmds:                       │
│   - build                    │     │       - go build -o bin/server  │
│   - run                      │     │   run:                          │
└──────────────────────────────┘     │     cmds:                       │
                                     │       - cp bin/server /opt/...  │
      定义"执行什么"                   │       - systemctl restart ...   │
      (任务名称和顺序)                 └─────────────────────────────────┘
                                           定义"怎么做"
                                           (具体命令)
```

### 执行流程

1. Webhook 触发 → 匹配 `name`
2. 读取 `taskfile` 字段 → 找到 `conf.d/deploy_saas.api.yaml` (从 example 复制)
3. 按 `tasks` 顺序执行 → 依次调用 Taskfile 中的 `build`、`run` 任务

## 快速开始

### 1. 创建应用配置

复制示例文件并修改：

```bash
cp webhookd.example.yaml webhookd.yaml
```

编辑 `webhookd.yaml`，添加需要自动部署的仓库：

```yaml
apps:
  - name: my-app                    # 应用唯一标识 (webhook 匹配)
    title: My Application           # 应用标题 (日志显示)
    repo_addr: git@gogs.example.com:user/repo.git
    repo_path: /mnt/www/my-app      # 仓库本地路径
    taskfile: deploy_my-app.yaml    # Taskfile 路径 (相对于 conf.d/)
    branches:                       # 监听的分支 (留空则监听所有)
      - main
    tags: []                        # 监听的标签 (留空则监听所有)
    commits_message_prefix: ""      # 提交信息前缀 (留空则触发所有)
    tasks:                          # 任务链 (按顺序执行)
      - build
      - run
```

### 2. 创建 Taskfile

复制示例文件并修改：

```bash
cp deploy_example.yaml deploy_my-app.yaml
```

编辑 `deploy_my-app.yaml`：

```yaml
version: '3'

tasks:
  build:
    desc: 构建项目
    cmds:
      - echo "Building..."
      - # 添加构建命令

  run:
    desc: 部署项目
    cmds:
      - echo "Deploying..."
      - # 添加部署命令
```

> **注意**: `webhookd.yaml` 和 `deploy_*.yaml` (非 example 文件) 已被 `.gitignore` 忽略，不会提交到 Git。

## webhookd.yaml 字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | 是 | 应用唯一标识，用于 webhook 匹配 |
| `title` | 否 | 应用标题，仅用于日志显示 |
| `repo_addr` | 否 | Git 仓库地址，用于 clone/pull |
| `repo_path` | 是 | 仓库本地路径，工作目录 |
| `taskfile` | 否 | Taskfile 路径 (相对于 conf.d/)，为空时使用 repo_path/Taskfile.yml |
| `branches` | 否 | 监听的分支列表，留空则监听所有分支 |
| `tags` | 否 | 监听的标签列表，留空则监听所有标签 |
| `commits_message_prefix` | 否 | 提交信息前缀，留空则触发所有 |
| `timeout` | 否 | 任务超时时间 (秒)，默认 300 |
| `events` | 否 | 监听的事件类型 (push, tag, pull_request) |
| `tasks` | 是 | 任务链，按顺序执行 |

## 环境变量说明

在项目根目录的 `.env` 文件中配置：

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `WEBHOOK_SECRET` | 是 | - | Webhook 签名验证密钥 |
| `WEBHOOK_PORT` | 否 | `8080` | HTTP 服务监听端口 |
| `APP_CONFIG_FILE` | 否 | `conf.d/webhookd.yaml` | 配置文件路径 |
| `LOGS_DIR` | 否 | `logs` | 日志目录路径 |
| `ENABLE_FILE_LOGGING` | 否 | `false` | 是否启用文件日志 |
| `RUN_ON_STARTUP` | 否 | `false` | 启动时执行所有应用任务链 |

## 目录结构

```
conf.d/
├── webhookd.example.yaml          # 应用配置示例
├── deploy_example.yaml            # Taskfile 通用模板
├── deploy_saas.api.example.yaml   # Go API 项目示例
├── deploy_saas.web.example.yaml   # Node.js 前端示例
├── deploy_saas.grpc.example.yaml  # Go gRPC 项目示例
├── webhook-handler.service        # systemd 服务文件
├── README.md                      # 本文件
└── TASKFILE-README.md             # Taskfile 使用说明
```
