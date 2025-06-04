BINARY = webhook-handler
APPS := $(shell grep -E "^  - name:" conf.d/webhookd.yaml | sed 's/.*name: //' 2>/dev/null)

.DEFAULT_GOAL := help

help:
	@echo ""
	@echo "初始化"
	@echo "  make init           初始化项目 (创建目录、.env文件、设置权限)"
	@echo ""
	@echo "编译"
	@echo "  make build          编译程序到 bin/ 目录"
	@echo ""
	@echo "运行 (开发调试)"
	@echo "  make run            前台运行程序 (Ctrl+C 停止)"
	@echo ""
	@echo "部署 (生产环境)"
	@echo "  make deploy         编译 + 安装 + 启动 systemd 服务"
	@echo "  make uninstall      停止服务 + 删除服务文件"
	@echo ""
	@echo "服务管理 (systemd)"
	@echo "  make start          启动 webhook-handler 服务"
	@echo "  make stop           停止 webhook-handler 服务"
	@echo "  make restart        重启 webhook-handler 服务"
	@echo "  make st             查看 webhook-handler 服务状态"
	@echo "  make logs           查看 webhook-handler 服务日志"
	@echo ""
	@echo "单个应用管理"
	@echo "  make start <app>    启动应用 (app: server, platform, grpc)"
	@echo "  make stop <app>     停止应用"
	@echo "  make restart <app>  重启应用"
	@echo "  make st <app>       查看应用状态"
	@echo "  make logs <app>     查看应用日志"
	@echo ""
	@echo "清理"
	@echo "  make cleanup        清理日志文件和临时文件"
	@echo ""

init:
	bash bin/init.sh

build:
	@mkdir -p bin
	go build -o bin/$(BINARY) -mod=vendor

run: build
	@if [ ! -f .env ]; then \
		echo "错误: .env 文件未找到"; \
		echo "运行: cp conf.d/.env.example .env && vim .env"; \
		exit 1; \
	fi
	./bin/$(BINARY)

deploy: build
	sudo bash bin/install.sh
	sudo systemctl restart webhook-handler
	@sleep 2
	@echo ""
	@echo "部署完成! 检查状态..."
	@sudo systemctl status webhook-handler --no-pager
	@echo ""
	@echo "查看日志: make logs"

uninstall:
	sudo bash bin/uninstall.sh

cleanup:
	bash bin/cleanup.sh

start: $(if $(filter $(APPS),$(MAKECMDGOALS)),app-start,service-start)
stop: $(if $(filter $(APPS),$(MAKECMDGOALS)),app-stop,service-stop)
restart: $(if $(filter $(APPS),$(MAKECMDGOALS)),app-restart,service-restart)
st: $(if $(filter $(APPS),$(MAKECMDGOALS)),app-status,service-status)
logs: $(if $(filter $(APPS),$(MAKECMDGOALS)),app-logs,service-logs)

service-start:
	@echo "正在启动 webhook-handler..."
	sudo systemctl start webhook-handler
	@sleep 1
	sudo systemctl status webhook-handler --no-pager

service-stop:
	@echo "正在停止 webhook-handler..."
	sudo systemctl stop webhook-handler

service-restart:
	@echo "正在重启 webhook-handler..."
	sudo systemctl restart webhook-handler
	@sleep 1
	sudo systemctl status webhook-handler --no-pager

service-status:
	sudo systemctl status webhook-handler --no-pager

service-logs:
	tail -f /opt/deploy/logs/webhookd.log 2>/dev/null || echo "无日志。检查: ls -la /opt/deploy/logs/"

app-start:
	@echo "正在启动 $(filter $(APPS),$(MAKECMDGOALS))..."

app-stop:
	@echo "正在停止 $(filter $(APPS),$(MAKECMDGOALS))..."

app-restart:
	@echo "正在重启 $(filter $(APPS),$(MAKECMDGOALS))..."

app-status:
	@echo "状态: $(filter $(APPS),$(MAKECMDGOALS)):"
	@tail -5 /opt/deploy/logs/$(filter $(APPS),$(MAKECMDGOALS)).log 2>/dev/null || echo "无日志。检查: ls -la /opt/deploy/logs/"

app-logs:
	@tail -f /opt/deploy/logs/$(filter $(APPS),$(MAKECMDGOALS)).log 2>/dev/null || echo "无日志。检查: ls -la /opt/deploy/logs/"

.PHONY: help init build run deploy uninstall cleanup \
        start stop restart st logs \
        service-start service-stop service-restart service-status service-logs \
        app-start app-stop app-restart app-status app-logs $(APPS)
