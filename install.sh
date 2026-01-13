#!/bin/bash

# 配置信息
APP_NAME="secure_chat"
INSTALL_DIR="/opt/secure_chat"
BIN_NAME="server_linux"
GITHUB_RAW_URL="https://raw.githubusercontent.com/KAYUKI-one/SecureChat/main"
DOWNLOAD_URL="https://github.com/KAYUKI-one/SecureChat/releases/download/latest/server_linux"

# 权限检查
if [ "$EUID" -ne 0 ]; then 
  echo "请使用 root 权限运行此脚本 (sudo bash install.sh)"
  exit
fi

echo "--- 开始安装 SecureChat Pro v5 服务端 ---"

# 1. 创建安装目录
mkdir -p $INSTALL_DIR
mkdir -p $INSTALL_DIR/data
cd $INSTALL_DIR

# 2. 下载服务端二进制文件 (从 Release 下载)
echo "正在从 GitHub 获取最新版本..."
curl -L -o $BIN_NAME $DOWNLOAD_URL
chmod +x $BIN_NAME

# 3. 检查证书是否存在
if [ ! -f "server.crt" ]; then
    echo "警告: 未检测到 server.crt 和 server.key。"
    echo "请手动将证书放入 $INSTALL_DIR 目录，否则服务无法启动。"
fi

# 4. 创建 Systemd 服务文件
cat <<EOF > /etc/systemd/system/secure-chat.service
[Unit]
Description=SecureChat Pro v5 Backend Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/$BIN_NAME
Restart=always
RestartSec=5
StandardOutput=append:$INSTALL_DIR/server.log
StandardError=append:$INSTALL_DIR/server.log

[Install]
WantedBy=multi-user.target
EOF

# 5. 创建便捷管理指令 (chatserver)
cat <<'EOF' > /usr/local/bin/chatserver
#!/bin/bash
case "$1" in
    start)
        systemctl start secure-chat
        echo "服务端已启动。"
        ;;
    stop)
        systemctl stop secure-chat
        echo "服务端已停止。"
        ;;
    restart)
        systemctl restart secure-chat
        echo "服务端已重启。"
        ;;
    log)
        tail -f /opt/secure_chat/server.log
        ;;
    status)
        systemctl status secure-chat
        ;;
    remove)
        read -p "确定要彻底删除服务和所有聊天数据吗？[y/N] " conf
        if [ "$conf" == "y" ]; then
            systemctl stop secure-chat
            systemctl disable secure-chat
            rm /etc/systemd/system/secure-chat.service
            rm -rf /opt/secure_chat
            rm /usr/local/bin/chatserver
            echo "已彻底卸载。"
        fi
        ;;
    *)
        echo "用法: chatserver {start|stop|restart|status|log|remove}"
        ;;
esac
EOF

chmod +x /usr/local/bin/chatserver

# 6. 加载并启动服务
systemctl daemon-reload
systemctl enable secure-chat
systemctl start secure-chat

echo "------------------------------------------------"
echo "安装完成！"
echo "你可以使用以下指令管理服务端："
echo "chatserver start   - 启动"
echo "chatserver stop    - 停止"
echo "chatserver log     - 查看日志"
echo "chatserver remove  - 彻底卸载"
echo "------------------------------------------------"