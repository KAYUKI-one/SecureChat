package core

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"chat/internal/crypto"
	"chat/internal/protocol"

	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
)

type Engine struct {
	DB           *sql.DB
	Conn         *websocket.Conn
	ChatKey      []byte
	Server       string
	MyID         string
	MyAvatarHash string
	TempDir      string
	mu           sync.Mutex

	// 回调接口
	OnNewMessage func(protocol.Message)
	OnStatus     func(string, bool)
	OnProgress   func(float64) // 进度百分比 (0.0 - 1.0)
}

// NewEngine 初始化引擎并确保 ChatTemp 目录及数据库结构
func NewEngine(serverAddr, userID, rawKey string) (*Engine, error) {
	exePath, _ := os.Executable()
	baseDir := filepath.Dir(exePath)
	tempDir := filepath.Join(baseDir, "ChatTemp")
	_ = os.MkdirAll(tempDir, 0755)

	dbPath := filepath.Join(tempDir, "client_vault.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// 确保表结构包含 tp 和 ah 字段，防止文件无法显示
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS chat_log (
		id INTEGER PRIMARY KEY,
		u TEXT, t TEXT, tm TEXT, h TEXT, n TEXT, ah TEXT, tp TEXT
	)`)
	if err != nil {
		return nil, err
	}

	// 自动迁移逻辑
	cols := []string{"tp", "ah"}
	for _, col := range cols {
		var c int
		_ = db.QueryRow("SELECT count(*) FROM pragma_table_info('chat_log') WHERE name=?", col).Scan(&c)
		if c == 0 {
			_, _ = db.Exec(fmt.Sprintf("ALTER TABLE chat_log ADD COLUMN %s TEXT DEFAULT ''", col))
		}
	}

	return &Engine{
		DB:      db,
		Server:  strings.TrimSpace(serverAddr),
		MyID:    userID,
		ChatKey: crypto.DeriveKey(rawKey),
		TempDir: tempDir,
	}, nil
}

func (e *Engine) Start() {
	go e.connectLoop()
}

func (e *Engine) connectLoop() {
	for {
		var lastID uint32
		_ = e.DB.QueryRow("SELECT id FROM chat_log ORDER BY id DESC LIMIT 1").Scan(&lastID)

		addr := e.Server
		if strings.HasPrefix(addr, "https://") {
			addr = "wss://" + addr[8:]
		} else if !strings.HasPrefix(addr, "ws") {
			addr = "wss://" + addr
		}
		if !strings.HasSuffix(addr, "/ws") {
			addr = strings.TrimSuffix(addr, "/") + "/ws"
		}

		dialer := websocket.Dialer{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			HandshakeTimeout: 10 * time.Second,
		}

		c, _, err := dialer.Dial(addr, nil)
		if err != nil {
			if e.OnStatus != nil { e.OnStatus("Offline - Retrying", false) }
			time.Sleep(3 * time.Second)
			continue
		}

		e.Conn = c
		if e.OnStatus != nil { e.OnStatus("Online", true) }

		// 发送同步指令
		_ = e.Conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("SYNC:%d", lastID)))

		e.receiveLoop()

		e.mu.Lock()
		if e.Conn != nil { e.Conn.Close(); e.Conn = nil }
		e.mu.Unlock()
		time.Sleep(1 * time.Second)
	}
}

func (e *Engine) receiveLoop() {
	for {
		_, packet, err := e.Conn.ReadMessage()
		if err != nil { return }
		if len(packet) <= 4 { continue }
		
		id := binary.BigEndian.Uint32(packet[0:4])
		plain, err := crypto.Decrypt(packet[4:], e.ChatKey)
		if err != nil { continue }

		var m protocol.Message
		_ = json.Unmarshal(plain, &m)
		m.ID = id

		_, _ = e.DB.Exec("INSERT OR IGNORE INTO chat_log (id, u, t, tm, h, n, ah, tp) VALUES (?,?,?,?,?,?,?,?)",
			m.ID, m.User, m.Text, m.Time, m.Hash, m.Name, m.AvatarHash, m.Type)
		
		if e.OnNewMessage != nil { e.OnNewMessage(m) }
	}
}

func (e *Engine) SendText(text string) {
	e.sendEncrypted(protocol.Message{
		Type: "text", User: e.MyID, Text: text, 
		Time: time.Now().Format("15:04"), AvatarHash: e.MyAvatarHash,
	})
}

func (e *Engine) SendFileMessage(name, hash string) {
	e.sendEncrypted(protocol.Message{
		Type: "file", User: e.MyID, Name: name, Hash: hash, 
		Time: time.Now().Format("15:04"), AvatarHash: e.MyAvatarHash,
	})
}

func (e *Engine) sendEncrypted(m protocol.Message) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Conn == nil { return }
	plain, _ := json.Marshal(m)
	cipher, _ := crypto.Encrypt(plain, e.ChatKey)
	_ = e.Conn.WriteMessage(websocket.BinaryMessage, cipher)
}

func (e *Engine) LoadCache() []protocol.Message {
	rows, err := e.DB.Query(`SELECT id, u, t, tm, h, n, ah, tp FROM chat_log ORDER BY id ASC`)
	if err != nil { return nil }
	defer rows.Close()

	var list []protocol.Message
	for rows.Next() {
		var m protocol.Message
		_ = rows.Scan(&m.ID, &m.User, &m.Text, &m.Time, &m.Hash, &m.Name, &m.AvatarHash, &m.Type)
		list = append(list, m)
	}
	return list
}

// --- 带百分比进度的上传 ---
func (e *Engine) UploadFile(name string, content []byte) (string, error) {
	enc, _ := crypto.Encrypt(content, e.ChatKey)
	hash := crypto.Hash(enc)
	total := int64(len(enc))

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	
	// 1. 秒传检查
	checkURL := fmt.Sprintf("%s/check?hash=%s", strings.TrimSuffix(e.Server, "/"), hash)
	if resp, err := client.Get(checkURL); err == nil && resp.StatusCode == http.StatusOK {
		if e.OnProgress != nil { e.OnProgress(1.0) }
		return hash, nil
	}

	// 2. 流式上传并监控进度
	bodyReader := bytes.NewReader(enc)
	progressReader := &teeReader{
		Reader: bodyReader,
		onRead: func(n int) {
			current, _ := bodyReader.Seek(0, io.SeekCurrent)
			if e.OnProgress != nil { e.OnProgress(float64(current) / float64(total)) }
		},
	}

	uploadURL := fmt.Sprintf("%s/upload?hash=%s", strings.TrimSuffix(e.Server, "/"), hash)
	req, _ := http.NewRequest("POST", uploadURL, progressReader)
	req.ContentLength = total
	_, err := client.Do(req)
	return hash, err
}

// --- 带百分比进度的下载 ---
func (e *Engine) DownloadFile(hash string) ([]byte, error) {
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	url := fmt.Sprintf("%s/download?hash=%s", strings.TrimSuffix(e.Server, "/"), hash)
	resp, err := client.Get(url)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	total := resp.ContentLength
	var current int64
	var result bytes.Buffer
	buf := make([]byte, 32*1024)

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			result.Write(buf[:n])
			current += int64(n)
			if e.OnProgress != nil && total > 0 {
				e.OnProgress(float64(current) / float64(total))
			}
		}
		if err == io.EOF { break }
		if err != nil { return nil, err }
	}
	return crypto.Decrypt(result.Bytes(), e.ChatKey)
}

// 进度拦截包装器
type teeReader struct {
	io.Reader
	onRead func(int)
}
func (t *teeReader) Read(p []byte) (int, error) {
	n, err := t.Reader.Read(p)
	t.onRead(n)
	return n, err
}