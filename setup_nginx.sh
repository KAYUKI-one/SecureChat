#!/bin/bash

# 配置变量
FAKE_SITE_DIR="/var/www/secure_proxy"
NGINX_CONF="/etc/nginx/conf.d/secure_chat.conf"
BACKEND_URL="https://127.0.0.1:8080" # 你的 Go 后端地址
WS_PATH="/api/v2/updates"           # 隐藏的 WebSocket 路径

# 检查权限
if [ "$EUID" -ne 0 ]; then 
  echo "请以 root 权限运行: sudo bash setup_nginx.sh"
  exit
fi

echo "--- 开始配置 Nginx 流量伪装系统 ---"

# 1. 安装 Nginx
if [ -f /etc/redhat-release ]; then
    yum install -y nginx openssl
else
    apt-get update && apt-get install -y nginx openssl
fi

# 2. 创建伪造的静态网页 (极简个人作品集模板)
mkdir -p $FAKE_SITE_DIR
cat <<EOF > $FAKE_SITE_DIR/index.html
<!DOCTYPE html>
<html>
<head>
    <title>Portfolio | Creative Developer</title>
    <style>
        body { background: #0f172a; color: #f8fafc; font-family: sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
        .container { text-align: center; border: 1px solid #1e293b; padding: 50px; border-radius: 20px; background: #1e293b; }
        h1 { color: #38bdf8; }
        p { color: #94a3b8; }
        .btn { display: inline-block; margin-top: 20px; padding: 10px 20px; background: #38bdf8; color: #0f172a; text-decoration: none; border-radius: 5px; font-weight: bold; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Under Construction</h1>
        <p>Welcome to my personal workspace. I'm currently working on some exciting Go-based projects.</p>
        <p>System Status: <span style="color: #4ade80;">Operational</span></p>
        <a href="#" class="btn">View My GitHub</a>
    </div>
</body>
</html>
EOF

# 3. 获取用户域名
read -p "请输入你的域名 (如果没有, 请直接按回车使用IP): " DOMAIN
if [ -z "$DOMAIN" ]; then
    DOMAIN="localhost"
fi

# 4. 生成 Nginx 配置文件
cat <<EOF > $NGINX_CONF
server {
    listen 443 ssl http2;
    server_name $DOMAIN;

    # 证书路径 (需确保 install.sh 里的证书在此处或手动修改)
    ssl_certificate     /opt/secure_chat/server.crt;
    ssl_certificate_key /opt/secure_chat/server.key;

    ssl_session_timeout 1d;
    ssl_session_cache shared:MozSSL:10m;
    ssl_protocols TLSv1.2 TLSv1.3;

    # 伪装层：访问根目录显示假网站
    location / {
        root $FAKE_SITE_DIR;
        index index.html;
    }

    # 隐藏层：真正的 WebSocket 入口
    location $WS_PATH {
        proxy_pass $BACKEND_URL/ws; 
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "Upgrade";
        proxy_set_header Host \$host;
        
        # 传递真实IP
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;

        # 忽略后端自签名证书报错
        proxy_ssl_verify off;
        
        # 超时设置
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}

# 自动重定向 80 到 443
server {
    listen 80;
    server_name $DOMAIN;
    return 301 https://\$host\$request_uri;
}
EOF

# 5. 测试并重启 Nginx
nginx -t && systemctl restart nginx
systemctl enable nginx

echo "------------------------------------------------"
echo "配置完成！"
echo "伪装网站地址: https://$DOMAIN"
echo "隐藏通讯接口: https://$DOMAIN$WS_PATH"
echo ""
echo "请在客户端 Server Address 填入: https://$DOMAIN$WS_PATH"
echo "------------------------------------------------"