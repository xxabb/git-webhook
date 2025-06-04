# Taskfile.yml 使用说明

## 概述

每个需要通过 webhook 自动部署的仓库，都需要在根目录放置一个 `Taskfile.yml` 文件，定义具体的构建和部署任务。

## 文件位置

Deploy 服务的 Taskfile 模板位于 `conf.d/` 目录，命名为 `deploy_[apps.name].yaml`：

```
conf.d/deploy_saas.server.yaml      # 对应 webhookd.yaml 中 name: saas.server
conf.d/deploy_saas.platform.yaml    # 对应 webhookd.yaml 中 name: saas.platform
conf.d/deploy_saas.grpc.yaml        # 对应 webhookd.yaml 中 name: saas.grpc
```

将对应的 Taskfile 复制到仓库根目录并重命名为 `Taskfile.yml`：

```bash
cp conf.d/deploy_saas.server.yaml /mnt/www/server/Taskfile.yml
cp conf.d/deploy_saas.platform.yaml /mnt/www/platform/Taskfile.yml
cp conf.d/deploy_saas.grpc.yaml /mnt/www/grpc/Taskfile.yml
```

## 任务定义

### 标准任务

每个仓库应实现以下标准任务：

| 任务名 | 用途 | 触发时机 |
|--------|------|----------|
| `build` | 构建项目 | 每次代码推送 |
| `run` | 重启/部署服务 | 构建完成后 |
| `publish` | 完整发布流程 | 手动触发或标签推送 |

### 可选任务

| 任务名 | 用途 |
|--------|------|
| `test` | 运行测试 |
| `clean` | 清理缓存 |
| `dev` | 启动开发服务器 |
| `proto` | 生成 protobuf 代码 |

## 配置示例

### PHP 项目 (Laravel)

```yaml
version: '3'

tasks:
  build:
    desc: 构建项目
    cmds:
      - docker exec php84 composer install --no-dev
      - docker exec php84 php artisan config:cache
      - docker exec php84 php artisan route:cache

  run:
    desc: 重启服务
    cmds:
      - docker exec php84 php artisan migrate --force
      - docker exec php84 php artisan queue:restart
```

### Node.js 项目 (Vue/React)

```yaml
version: '3'

tasks:
  build:
    desc: 构建前端
    cmds:
      - pnpm install --frozen-lockfile
      - pnpm build

  run:
    desc: 部署前端
    cmds:
      - rm -rf /var/www/app/dist
      - cp -r dist /var/www/app/
```

### Go 项目

```yaml
version: '3'

tasks:
  build:
    desc: 构建 Go 项目
    cmds:
      - go mod download
      - CGO_ENABLED=0 GOOS=linux go build -o bin/server ./cmd/server

  run:
    desc: 重启服务
    cmds:
      - systemctl restart my-service
```

## Webhook 触发流程

```
1. 代码推送到 Gogs/Gitea
2. Webhook 发送到 Deploy 服务
3. Deploy 服务验证签名
4. Deploy 服务匹配仓库配置
5. Deploy 服务执行任务链 (按 webhookd.yaml 中的 tasks 顺序)
   - 执行 build 任务
   - 执行 run 任务
6. 返回 200 OK
```

## 环境变量

Taskfile 中可以使用以下环境变量：

- `{{.USER_WORKING_DIR}}` - 仓库根目录
- `{{.TASKFILE_DIR}}` - Taskfile 所在目录

## 调试

手动测试 Taskfile：

```bash
cd /mnt/www/server
task --list          # 查看所有任务
task build           # 手动执行 build 任务
task build --dry     # 模拟执行，不实际运行
```

## 注意事项

1. Taskfile 必须放在仓库根目录，文件名为 `Taskfile.yml`
2. 任务命令在仓库根目录下执行
3. 任务按 webhookd.yaml 中定义的顺序依次执行
4. 某个任务失败会中止后续任务执行
5. 所有输出会记录到 `/opt/deploy/logs/{app_name}.log`
