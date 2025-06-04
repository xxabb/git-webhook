#!/bin/bash

# Webhook Handler 服务安装脚本
# 用法: sudo bash bin/install.sh

set -e

INSTALL_DIR="/opt/deploy"
SERVICE_NAME="webhook-handler"

echo "正在安装 ${SERVICE_NAME} 到 ${INSTALL_DIR}..."

# 创建安装目录
sudo mkdir -p "${INSTALL_DIR}"

# 复制二进制文件
echo "正在复制二进制文件..."
sudo cp -f bin/webhook-handler "${INSTALL_DIR}/"

# 复制配置文件
echo "正在复制配置文件..."
sudo mkdir -p "${INSTALL_DIR}/conf.d"
sudo cp -f conf.d/webhookd.yaml "${INSTALL_DIR}/conf.d/"
sudo cp -f conf.d/deploy_*.yaml "${INSTALL_DIR}/conf.d/"

# 复制 .env 文件 (如果存在)
if [ -f .env ]; then
    echo "正在复制 .env 文件..."
    sudo cp -f .env "${INSTALL_DIR}/"
    sudo chmod 600 "${INSTALL_DIR}/.env"
else
    echo "警告: .env 文件未找到。请手动创建 ${INSTALL_DIR}/.env。"
    echo "参考 .env.example。"
fi

# 创建日志目录
echo "正在创建日志目录..."
sudo mkdir -p "${INSTALL_DIR}/logs"
sudo chmod 755 "${INSTALL_DIR}/logs"

# 复制 systemd 服务文件
echo "正在安装 systemd 服务..."
sudo cp -f conf.d/${SERVICE_NAME}.service /etc/systemd/system/

# 重新加载 systemd
echo "正在重新加载 systemd..."
sudo systemctl daemon-reload

echo ""
echo "安装完成!"
echo ""
echo "启用并启动服务:"
echo "  sudo systemctl enable ${SERVICE_NAME}"
echo "  sudo systemctl start ${SERVICE_NAME}"
echo ""
echo "检查状态:"
echo "  sudo systemctl status ${SERVICE_NAME}"
echo ""
echo "查看日志:"
echo "  sudo journalctl -u ${SERVICE_NAME} -f"
