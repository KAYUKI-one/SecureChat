package main

import (
	"context"
	"crypto/sha256" // 引入用于计算识别码
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"chat/internal/core"
	"chat/internal/protocol"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SavedConfig 存储在 ChatTemp/config.json
type SavedConfig struct {
	Server     string `json:"server"`
	UID        string `json:"uid"`
	UserPass   string `json:"user_pass"` // 身份密码
	Key        string `json:"key"`
	AvatarHash string `json:"avatar"`
}

type App struct {
	ctx        context.Context
	engine     *core.Engine
	tempDir    string // ChatTemp 路径
	avatarDir  string // ChatTemp/avatars 路径
	configFile string // ChatTemp/config.json 路径
}

// NewApp 初始化 App 实例并配置本地存储路径
func NewApp() *App {
	exePath, _ := os.Executable()
	baseDir := filepath.Dir(exePath)
	
	tempDir := filepath.Join(baseDir, "ChatTemp")
	avatarDir := filepath.Join(tempDir, "avatars")
	
	// 确保本地文件夹存在
	_ = os.MkdirAll(avatarDir, 0755)

	return &App{
		tempDir:    tempDir,
		avatarDir:  avatarDir,
		configFile: filepath.Join(tempDir, "config.json"),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// generateTripcode 生成识别码后缀 (例如: Rosemary -> Rosemary#a1b2c3)
func generateTripcode(pass string) string {
	if pass == "" {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(pass + "id_salt_v5"))
	hash := fmt.Sprintf("%x", h.Sum(nil))
	return "#" + hash[:6]
}

// CheckAutoLogin 供前端启动时检查是否直接进入
func (a *App) CheckAutoLogin() map[string]string {
	data, err := os.ReadFile(a.configFile)
	if err != nil {
		return nil
	}
	var cfg SavedConfig
	_ = json.Unmarshal(data, &cfg)
	if cfg.Server != "" {
		return map[string]string{
			"server": cfg.Server,
			"uid":    cfg.UID,
			"upass":  cfg.UserPass,
			"key":    cfg.Key,
			"avatar": cfg.AvatarHash,
		}
	}
	return nil
}

// GetMyFullID 供前端获取带后缀的完整 ID（用于判断消息是不是自己发的）
func (a *App) GetMyFullID() string {
	if a.engine != nil {
		return a.engine.MyID
	}
	return ""
}

// Connect 初始化引擎并连接，加入 upass 处理
func (a *App) Connect(srv, id, upass, key, avatar string) string {
	// 核心：计算身份识别码，拼接到 ID 后面发送给引擎
	tripcode := generateTripcode(upass)
	finalID := id + tripcode

	eng, err := core.NewEngine(srv, finalID, key)
	if err != nil {
		return err.Error()
	}
	a.engine = eng
	a.engine.MyAvatarHash = avatar

	// 绑定新消息事件
	a.engine.OnNewMessage = func(m protocol.Message) {
		runtime.EventsEmit(a.ctx, "on_new_msg", m)
	}

	// 绑定连接状态事件
	a.engine.OnStatus = func(s string, online bool) {
		runtime.EventsEmit(a.ctx, "on_status_change", map[string]interface{}{
			"status": s, "online": online,
		})
	}

	// 绑定百分比进度事件
	a.engine.OnProgress = func(p float64) {
		runtime.EventsEmit(a.ctx, "on_transfer_progress", p*100)
	}

	// 持久化本次登录凭证
	cfg := SavedConfig{
		Server:     srv,
		UID:        id,
		UserPass:   upass,
		Key:        key,
		AvatarHash: avatar,
	}
	cfgBytes, _ := json.Marshal(cfg)
	_ = os.WriteFile(a.configFile, cfgBytes, 0644)

	a.engine.Start()
	return "success"
}

// GetAvatar 实现基于磁盘缓存的加载逻辑
func (a *App) GetAvatar(hash string) string {
	if hash == "" { return "" }

	localPath := filepath.Join(a.avatarDir, hash+".cache")
	if data, err := os.ReadFile(localPath); err == nil {
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	}

	if a.engine == nil { return "" }
	// 严格使用你原本引擎里的 DownloadFile
	data, err := a.engine.DownloadFile(hash) 
	if err != nil { return "" }

	_ = os.WriteFile(localPath, data, 0644)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

func (a *App) SendMessage(text string) {
	if a.engine != nil {
		a.engine.SendText(text)
	}
}

func (a *App) GetHistory() []protocol.Message {
	if a.engine != nil {
		return a.engine.LoadCache()
	}
	return nil
}

func (a *App) SelectAndUpload() {
	if a.engine == nil { return }
	file, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Select File"})
	if err != nil || file == "" { return }

	go func() {
		data, _ := os.ReadFile(file)
		info, _ := os.Stat(file)
		hash, err := a.engine.UploadFile(info.Name(), data)
		if err == nil {
			a.engine.SendFileMessage(info.Name(), hash)
		}
	}()
}

func (a *App) DownloadFile(hash, name string) {
	if a.engine == nil { return }
	save, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{DefaultFilename: name})
	if err != nil || save == "" { return }

	go func() {
		data, err := a.engine.DownloadFile(hash)
		if err == nil {
			_ = os.WriteFile(save, data, 0644)
		}
	}()
}

func (a *App) UploadAvatar() string {
	if a.engine == nil { return "" }
	file, _ := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Update Avatar",
		Filters: []runtime.FileFilter{{DisplayName: "Images", Pattern: "*.png;*.jpg;*.jpeg"}},
	})
	if file == "" { return "" }

	data, _ := os.ReadFile(file)
	hash, _ := a.engine.UploadFile("avatar", data)
	
	localPath := filepath.Join(a.avatarDir, hash+".cache")
	_ = os.WriteFile(localPath, data, 0644)

	a.engine.MyAvatarHash = hash
	
	cfgData, _ := os.ReadFile(a.configFile)
	var cfg SavedConfig
	_ = json.Unmarshal(cfgData, &cfg)
	cfg.AvatarHash = hash
	newCfg, _ := json.Marshal(cfg)
	_ = os.WriteFile(a.configFile, newCfg, 0644)

	return hash
}

func (a *App) WipeData() {
	if a.engine != nil && a.engine.DB != nil {
		a.engine.DB.Close()
	}
	_ = os.RemoveAll(a.tempDir)
}

func (a *App) UpdateAvatarHash(hash string) {
	if a.engine != nil {
		a.engine.MyAvatarHash = hash
	}
	data, err := os.ReadFile(a.configFile)
	if err != nil { return }
	
	var cfg SavedConfig
	json.Unmarshal(data, &cfg)
	cfg.AvatarHash = hash
	
	newCfg, _ := json.Marshal(cfg)
	_ = os.WriteFile(a.configFile, newCfg, 0644)
}