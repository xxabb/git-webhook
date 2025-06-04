#!/bin/bash

# Webhook Handler 清理脚本
# 用法: bash bin/cleanup.sh

set -e

echo "正在清理..."

# 停止并禁用服务
SERVICE_NAME="webhook-handler"
if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    echo "正在停止服务..."
    sudo systemctl stop "${SERVICE_NAME}"
fi
if systemctl is-enabled --quiet "${SERVICE_NAME}" 2>/dev/null; then
    echo "正在禁用服务..."
    sudo systemctl disable "${SERVICE_NAME}"
fi

# 删除 systemd 服务文件
if [ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]; then
    echo "正在删除服务文件..."
    sudo rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
    sudo systemctl daemon-reload
fi

# 清理日志文件
if [ -d "logs" ]; then
    echo "正在删除日志文件..."
    rm -f logs/*.log
fi

# 清理临时文件
echo "正在删除临时文件..."
rm -rf /tmp/task-* 2>/dev/null || true

# 清理编译产物
if [ -f "bin/webhook-handler" ]; then
    echo "正在删除二进制文件..."
    rm -f bin/webhook-handler
fi

echo ""
echo "清理完成!"
