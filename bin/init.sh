#!/bin/bash

# Webhook Handler 初始化脚本
# 用法: bash bin/init.sh

set -e

echo "正在初始化 Webhook Handler..."

# 创建目录结构
echo "正在创建目录..."
mkdir -p logs conf.d

# 设置目录权限
chmod 755 logs conf.d

# 创建 .env 文件 (如果不存在)
if [ ! -f .env ]; then
    if [ -f conf.d/.env.example ]; then
        echo "正在从模板创建 .env..."
        cp conf.d/.env.example .env
        chmod 600 .env
        echo "请编辑 .env 并设置 WEBHOOK_SECRET"
    else
        echo "警告: conf.d/.env.example 未找到"
    fi
else
    echo ".env 已存在"
fi

# 复制 systemd 服务文件
if [ -f conf.d/webhook-handler.service ]; then
    echo "正在复制 systemd 服务文件..."
    sudo cp -f conf.d/webhook-handler.service /etc/systemd/system/
    sudo systemctl daemon-reload
    echo "服务文件已复制到 /etc/systemd/system/"
else
    echo "警告: conf.d/webhook-handler.service 未找到"
fi

# 设置配置文件权限
echo "正在设置文件权限..."
chmod 644 conf.d/*.yaml 2>/dev/null || true
chmod 644 conf.d/*.service 2>/dev/null || true

# 设置脚本权限
chmod +x bin/init.sh
chmod +x bin/install.sh
chmod +x bin/uninstall.sh
chmod +x bin/cleanup.sh

echo ""
echo "初始化完成!"
echo ""
echo "下一步:"
echo "  1. 编辑 .env 并设置 WEBHOOK_SECRET"
echo "  2. 编辑 conf.d/webhookd.yaml 配置应用"
echo "  3. 运行: make build"
echo "  4. 运行: make deploy"
