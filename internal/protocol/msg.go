package protocol

// Message 是聊天消息的统一定义
type Message struct {
	ID         uint32 `json:"id"` // 服务端唯一递增ID
	Type       string `json:"tp"` // text (文本), file (文件), profile (配置同步)
	User       string `json:"u"`  // 发送者ID
	Text       string `json:"t"`  // 消息文本
	Hash       string `json:"h"`  // 文件在服务器上的加密哈希
	Name       string `json:"n"`  // 文件名
	Time       string `json:"tm"` // 发送时间 (15:04)
	AvatarHash string `json:"ah"` // 发送者的头像哈希
}

// LocalConfig 是存在用户电脑上的加密配置结构
type LocalConfig struct {
	Server  string `json:"s"`
	ChatKey string `json:"k"`
	MyID    string `json:"id"`
}