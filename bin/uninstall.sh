#!/bin/bash

# Webhook Handler 服务卸载脚本
# 用法: bash bin/uninstall.sh

set -e

SERVICE_NAME="webhook-handler"

echo "正在卸载 ${SERVICE_NAME}..."

# 停止服务
if systemctl is-active --quiet "${SERVICE_NAME}"; then
    echo "正在停止 ${SERVICE_NAME}..."
    sudo systemctl stop "${SERVICE_NAME}"
fi

# 禁用开机自启
if systemctl is-enabled --quiet "${SERVICE_NAME}" 2>/dev/null; then
    echo "正在禁用 ${SERVICE_NAME}..."
    sudo systemctl disable "${SERVICE_NAME}"
fi

# 删除服务文件
if [ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]; then
    echo "正在删除服务文件..."
    sudo rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
fi

# 重载 systemd
echo "正在重新加载 systemd..."
sudo systemctl daemon-reload

echo ""
echo "${SERVICE_NAME} 卸载成功!"
echo ""
echo "注意: /opt/deploy/ 目录未被删除。"
echo "手动删除: sudo rm -rf /opt/deploy/"
