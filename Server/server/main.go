package main

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite" // 纯 Go 实现的 SQLite 驱动
)

var (
	// 数据存储根目录
	DataDir = "./data"
	db      *sql.DB
	clients = make(map[*websocket.Conn]bool)
	lock    = sync.Mutex{}
	
	// WebSocket 升级配置
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

func init() {
	// 1. 确保上传文件夹存在
	os.MkdirAll(filepath.Join(DataDir, "uploads"), 0755)

	// 2. 初始化数据库
	var err error
	dbPath := filepath.Join(DataDir, "vault_v5.db")
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal("无法打开数据库:", err)
	}

	// 3. 创建消息表：id 是增量同步的关键
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		data BLOB,
		time DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatal("数据库初始化失败:", err)
	}
	log.Println("数据库就绪:", dbPath)
}

func main() {
	// 路由注册
	http.HandleFunc("/ws", handleWS)             // 聊天与增量同步
	http.HandleFunc("/check", handleCheck)       // 秒传预检
	http.HandleFunc("/upload", handleUpload)     // 加密上传
	http.HandleFunc("/download", handleDownload) // 加密下载

	log.Println("--- SecureChat 后端引擎 v5.0 ---")
	log.Println("运行状态: 监听 WSS 端口 :8080")

	// 必须确保目录下有 server.crt 和 server.key
	err := http.ListenAndServeTLS(":8080", "server.crt", "server.key", nil)
	if err != nil {
		log.Fatal("TLS 启动失败，请检查证书文件:", err)
	}
}

// handleWS 处理连接与增量同步
func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	log.Printf("新终端接入: %s", conn.RemoteAddr())

	// A. 读取客户端发来的第一个包，格式为 "SYNC:{LastID}"
	_, firstMsg, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var lastID uint32
	fmt.Sscanf(string(firstMsg), "SYNC:%d", &lastID)
	log.Printf("客户端请求增量同步，起点 ID: %d", lastID)

	// B. 增量推送历史记录
	rows, _ := db.Query("SELECT id, data FROM messages WHERE id > ? ORDER BY id ASC", lastID)
	count := 0
	for rows.Next() {
		var id uint32
		var d []byte
		rows.Scan(&id, &d)

		// 封装数据包：[4字节ID][加密内容]
		packet := make([]byte, 4+len(d))
		binary.BigEndian.PutUint32(packet[0:4], id)
		copy(packet[4:], d)

		conn.WriteMessage(websocket.BinaryMessage, packet)
		count++
	}
	rows.Close()
	log.Printf("同步完成，已补发 %d 条消息", count)

	// C. 注册到在线广播列表
	lock.Lock()
	clients[conn] = true
	lock.Unlock()

	// D. 监听客户端新消息并广播
	for {
		_, rawData, err := conn.ReadMessage()
		if err != nil {
			lock.Lock()
			delete(clients, conn)
			lock.Unlock()
			break
		}

		// 1. 存入数据库
		res, err := db.Exec("INSERT INTO messages (data) VALUES (?)", rawData)
		if err != nil {
			log.Println("存储失败:", err)
			continue
		}
		newID, _ := res.LastInsertId()

		// 2. 封装广播包
		packet := make([]byte, 4+len(rawData))
		binary.BigEndian.PutUint32(packet[0:4], uint32(newID))
		copy(packet[4:], rawData)

		// 3. 执行广播
		broadcast(packet)
	}
}

func broadcast(packet []byte) {
	lock.Lock()
	defer lock.Unlock()
	for client := range clients {
		err := client.WriteMessage(websocket.BinaryMessage, packet)
		if err != nil {
			client.Close()
			delete(clients, client)
		}
	}
}

// handleCheck 秒传检查
func handleCheck(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	path := filepath.Join(DataDir, "uploads", hash)
	if _, err := os.Stat(path); err == nil {
		w.WriteHeader(http.StatusOK) // 存在，可以秒传
		log.Println("秒传触发:", hash)
	} else {
		w.WriteHeader(http.StatusNotFound) // 不存在，需要上传
	}
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	path := filepath.Join(DataDir, "uploads", hash)
	
	// 再次检查防止并发重复写入
	if _, err := os.Stat(path); err == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	f, err := os.Create(path)
	if err != nil {
		http.Error(w, "磁盘写入错误", 500)
		return
	}
	defer f.Close()

	io.Copy(f, r.Body)
	w.WriteHeader(http.StatusCreated)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	path := filepath.Join(DataDir, "uploads", hash)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		http.Error(w, "文件不存在", 404)
		return
	}
	http.ServeFile(w, r, path)
}