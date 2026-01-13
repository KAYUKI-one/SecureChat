/**
 * SecureChat Pro v5 - 旗舰版前端逻辑
 * 核心：身份确权码、百分比进度、头像自愈、磁盘缓存
 */

const ui = {
    myID: "",           // 用户输入的原始 ID (如: Rosemary)
    myFullID: "",       // 带后缀的最终 ID (如: Rosemary#a1b2c3)
    myAvatarHash: "",   // 当前用户的头像哈希
    avatarCache: {},    // 内存 Base64 缓存

    init() {
        console.log("UI Initializing...");

        // 1. 监听来自 Go 的新消息 (实时推送或历史同步)
        window.runtime.EventsOn("on_new_msg", (m) => {
            this.appendBubble(m);
        });

        // 2. 监听连接状态变化
        window.runtime.EventsOn("on_status_change", (s) => {
            const statusEl = document.getElementById('net-status');
            if (statusEl) {
                statusEl.innerText = s.status;
                statusEl.style.color = s.online ? "#00ff64" : "#ff4b4b";
            }
        });

        // 3. 监听传输百分比 (%)
        window.runtime.EventsOn("on_transfer_progress", (percent) => {
            const pBox = document.getElementById('progress-container');
            const pBar = document.getElementById('prog-bar');
            const pText = document.getElementById('prog-text');
            
            pBox.style.display = 'block';
            pBar.style.width = percent + "%";
            pText.innerText = Math.floor(percent) + "%";

            // 完成后延迟隐藏
            if (percent >= 100) {
                setTimeout(() => {
                    pBox.style.display = 'none';
                    pText.innerText = "";
                }, 1500);
            }
        });

        // 4. 输入框回车发送
        document.getElementById('msg-input').addEventListener('keypress', (e) => {
            if (e.key === 'Enter') this.sendText();
        });

        // 5. 启动自检测：ChatTemp 自动恢复
        setTimeout(() => this.checkSavedLogin(), 200);
    },

    async checkSavedLogin() {
        try {
            const saved = await window.go.main.App.CheckAutoLogin();
            if (saved && saved.server) {
                console.log("ChatTemp 配置发现，自动登录中...");
                document.getElementById('srv-addr').value = saved.server;
                document.getElementById('user-id').value = saved.uid;
                document.getElementById('user-pass').value = saved.upass; 
                document.getElementById('chat-key').value = saved.key;
                this.myAvatarHash = saved.avatar;
                this.connect();
            }
        } catch (e) {
            console.log("无保存的会话");
        }
    },

    async connect() {
        const srv = document.getElementById('srv-addr').value;
        const uid = document.getElementById('user-id').value;
        const upass = document.getElementById('user-pass').value;
        const key = document.getElementById('chat-key').value;

        if (!uid || !key || !upass) return alert("All fields are required!");

        // 调用 Go 接口建立连接 (传入 5 个参数)
        const res = await window.go.main.App.Connect(srv, uid, upass, key, this.myAvatarHash);
        
        if (res === "success") {
            this.myID = uid;
            // 关键：从后端拿回带 # 后缀的完整 ID
            this.myFullID = await window.go.main.App.GetMyFullID(); 
            
            document.getElementById('login-screen').style.display = 'none';
            document.getElementById('display-id').innerText = this.myFullID; 
            
            // 初始渲染侧边栏头像 (磁盘缓存优先)
            this.renderAvatar(this.myAvatarHash, 'my-avatar', true);

            // 加载历史记录
            const msgs = await window.go.main.App.GetHistory();
            const list = document.getElementById('message-list');
            list.innerHTML = "";
            if (msgs) {
                msgs.forEach(m => this.appendBubble(m));

                // --- 头像自愈逻辑：从历史消息找回自己的 ah ---
                if (!this.myAvatarHash) {
                    console.log("正在从历史记录尝试找回身份...");
                    for (let i = msgs.length - 1; i >= 0; i--) {
                        // 匹配完整的带后缀 ID
                        if (msgs[i].u === this.myFullID && msgs[i].ah) {
                            this.myAvatarHash = msgs[i].ah;
                            this.renderAvatar(this.myAvatarHash, 'my-avatar', true);
                            window.go.main.App.UpdateAvatarHash(this.myAvatarHash);
                            break;
                        }
                    }
                }
            }
        } else {
            alert("Connection Error: " + res);
        }
    },

    appendBubble(m) {
        const container = document.getElementById('message-list');
        // 判断是否为自己：严格匹配带后缀的 FullID
        const isMe = m.u === this.myFullID;
        const bubbleId = `msg-${m.id || Math.random().toString(36).substr(2, 9)}`;

        const wrapper = document.createElement('div');
        wrapper.className = `msg-wrapper ${isMe ? 'own' : 'other'}`;
        wrapper.id = bubbleId;

        let content = m.t;
        if (m.tp === "file") {
            content = `
                <div class="file-card">
                    <span>📁 ${m.n}</span>
                    <button onclick="ui.download('${m.h}', '${m.n}')">DOWNLOAD</button>
                </div>`;
        }

        // 渲染结构：先出占位圆圈
        wrapper.innerHTML = `
            <div class="avatar-box">
                <div class="avatar-placeholder">${m.u.charAt(0).toUpperCase()}</div>
            </div>
            <div class="bubble">
                ${!isMe ? `<div class="msg-user">${m.u}</div>` : ''}
                <div class="msg-content">${content}</div>
                <div class="msg-time">${m.tm}</div>
            </div>
        `;

        container.appendChild(wrapper);
        container.scrollTop = container.scrollHeight;

        // 异步加载头像 (内存 -> 磁盘 -> 网络)
        if (m.ah) {
            this.renderAvatar(m.ah, bubbleId, false);
        }
    },

    async renderAvatar(hash, elementId, isSidebar) {
        if (!hash) return;
        let b64 = "";
        if (this.avatarCache[hash]) {
            b64 = this.avatarCache[hash];
        } else {
            b64 = await window.go.main.App.GetAvatar(hash);
            if (b64) this.avatarCache[hash] = b64;
        }

        if (b64) {
            const target = isSidebar ? 
                document.getElementById(elementId) : 
                document.querySelector(`#${elementId} .avatar-box`);
            if (target) {
                target.innerHTML = `<img src="${b64}" class="img-avatar">`;
            }
        }
    },

    sendText() {
        const el = document.getElementById('msg-input');
        if (!el.value) return;
        window.go.main.App.SendMessage(el.value);
        el.value = "";
    },

    async changeAvatar() {
        const hash = await window.go.main.App.UploadAvatar();
        if (hash) {
            this.myAvatarHash = hash;
            this.renderAvatar(hash, 'my-avatar', true);
        }
    },

    download(h, n) { window.go.main.App.DownloadFile(h, n); },
    selectFile() { window.go.main.App.SelectAndUpload(); },
    wipe() {
        if (confirm("Wipe all local data?")) {
            window.go.main.App.WipeData();
            location.reload();
        }
    }
};

window.ui = ui;
window.addEventListener('load', () => setTimeout(() => ui.init(), 100));