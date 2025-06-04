# Webhook Handler

轻量级 Git Webhook 触发器，接收 Gogs/Gitea 的 webhook 推送，通过 Task 执行部署脚本。

## 快速开始

```bash
# 1. 初始化项目
make init

# 2. 编辑配置
vim .env                    # 设置 WEBHOOK_SECRET
vim conf.d/webhookd.yaml    # 配置应用和任务链

# 3. 编译运行
make build
make run
```

## 部署

```bash
# 编译 + 安装 + 启动服务
make deploy

# 查看状态
make st

# 查看日志
make logs
```

## 命令

```bash
make              # 显示帮助
make init         # 初始化项目
make build        # 编译程序
make run          # 前台运行 (开发调试)
make deploy       # 部署到生产环境
make uninstall    # 卸载服务
make cleanup      # 清理日志

make start        # 启动服务
make stop         # 停止服务
make restart      # 重启服务
make st           # 查看状态
make logs         # 查看日志

make start <app>  # 启动应用 (server, platform, grpc)
make logs <app>   # 查看应用日志
```

## 配置

### 环境变量 (.env)

```bash
WEBHOOK_SECRET=your-secret    # Webhook 签名密钥 (必填)
WEBHOOK_PORT=8080             # 监听端口
APP_CONFIG_FILE=conf.d/webhookd.yaml  # 配置文件路径
LOGS_DIR=/opt/deploy/logs     # 日志目录
```

### 应用配置 (conf.d/webhookd.yaml)

```yaml
apps:
  - name: my-app                    # 应用唯一标识
    title: My Application           # 应用标题
    repo_addr: git@gogs.example.com:user/repo.git
    repo_path: /mnt/www/my-app      # 仓库路径
    taskfile: deploy_my-app.yaml    # Taskfile 路径
    branch: main                    # 监听分支
    tasks:                          # 任务链
      - build
      - run
```

### Taskfile (conf.d/deploy_*.yaml)

```yaml
version: '3'

tasks:
  build:
    cmds:
      - echo "Building..."
      - # 构建命令

  run:
    cmds:
      - echo "Deploying..."
      - # 部署命令
```

## Webhook 配置

在 Gogs/Gitea 中配置：

- **URL**: `http://your-server:8080/webhook`
- **Secret**: 与 `.env` 中的 `WEBHOOK_SECRET` 一致
- **Content Type**: `application/json`
- **Trigger**: Push Events

## nginx 配置

用于从容器内部访问宿主机

```yml
services:
  nginx:
    ...
+    extra_hosts:
+      - "host.docker.internal:host-gateway"
+    networks:
+      backend:
+        ipv4_address: ${NGINX_IP}
```

proxy

`nginx/conf.d/webhook.conf`

```yml
server {
    listen 443 ssl http2;
+    server_name <webhook.github.com>;
+    ssl_certificate ssl/github.com.crt;
+    ssl_certificate_key ssl/github.com.key;
    # HSTS 安全头
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;

    location / {
+        proxy_pass http://host.docker.internal:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket 支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
    # 调整上传文件大小限制（按需修改）
    client_max_body_size 50m;
}
```

## 目录结构

```
git-webhook/
├── main.go                        # 源代码
├── go.mod / go.sum                # Go 依赖
├── Makefile                       # 构建脚本
├── .env.example                   # 环境变量示例
├── bin/                           # 脚本和二进制
│   ├── init.sh
│   ├── install.sh
│   ├── uninstall.sh
│   └── cleanup.sh
├── conf.d/                        # 配置文件
│   ├── webhookd.example.yaml      # 应用配置示例
│   ├── webhookd.yaml              # 应用配置 (本地)
│   ├── deploy_*.example.yaml      # Taskfile 示例
│   └── webhook-handler.service    # systemd 服务文件
└── logs/                          # 日志 (运行时生成)
```

## License

MIT
