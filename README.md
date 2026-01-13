## v0.0.1版本说明
这是一个比较简易的匿名加密聊天工具，提供了客户端和服务端，代码由谷歌aistudio编写，本人只做审计和修改工作。加密方法采用XChaCha20，服务端看不见消息内容，但是可以云端存储。支持文件上传下载。
## 使用教程（服务端）

下载release里带有server_linux字样的文件到您的linux服务器，您可以通过以下命令：
```
curl -sSL https://raw.githubusercontent.com/KAYUKI-one/SecureChat/main/install.sh | sudo bash
```
随后，你可以自行采用何种方式隐藏传输特征，此处给出nginx静态网页伪装方案（信息本身加密，但是传输过程会产生特征）
```
curl -sSL https://raw.githubusercontent.com/KAYUKI-one/SecureChat/main/setup_nginx.sh | sudo bash
```
上述一键脚本在您并未自行安装nginx或一些代理内核的时候可以使用。
如果您已经安装了nginx，或者使用了一些占用特殊端口的代理内核，您可以考虑：

1. 登录 aaPanel，进入 **Website** 页面。
2. 找一个你已经配置好 **SSL (HTTPS)** 的网站（或者新建一个，并申请好 Let's Encrypt 证书）。
3. 确保该网站可以通过 https://你的域名 正常访问。
4. 1. 在 aaPanel 的网站列表点击 **Conf (设置)**。
5. 在左侧菜单选择 **Config (配置文件)**。
6. 在 server { ... } 区域内，access_log 下方，插入以下这段代码：
```
# 入口：外人访问这里只会看到 404 或被转发
location /api/v2/updates {
    proxy_pass https://127.0.0.1:8080/ws; # 转发到 Go 后端
    
    # WebSocket 核心配置
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "Upgrade";
    
    # 隐藏真实路径，防探测
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

    # 如果 Go 后端使用的是自签名证书，必须开启此项
    proxy_ssl_verify off;

    # 延长超时时间，防止聊天断线
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```
对于上述方案，您的客户端 **Server Address** 中填入:
https://你的域名/api/v2/updates

## 客户端
客户端不应当有使用教程，因为它十分简洁，只是有几点需要说明：  
1.服务器地址，请您自行根据服务端安装方案选择填入的服务器地址  
2.用户名和密码：用户名不唯一，但是用户名和密码二者会在您的用户名后产生唯一后缀，所以这是识别您身份的唯一方式。云端不存储任何密码。  
3.关于安全：当前版本没有本地加密，只实现了云端数据加密和传输加密，本地部分数据库内容是明文，自行斟酌。  
4.关于密钥：密钥是决定当前服务器下您进入的群组。每一个密钥都会动态生成一个盐值，加密您的数据。所以密钥不同看到的数据也不同，从而实现了无限的群组。请妥善保管您和组员的密钥。  

补充（各版本缺点，自用）：  
v0.0.1 不安全，UI简陋，不能复制消息（反而更安全？）  

## 声明
本程序仅供学习Go语言后端和Wails前端，请勿用于非法用途！（而且，都非法了你干嘛用俺的程序？）
