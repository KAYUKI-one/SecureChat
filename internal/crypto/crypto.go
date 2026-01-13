package crypto

import (
	"crypto/rand"
	"crypto/sha256" // 引入 SHA256 用于动态盐生成
	"fmt"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
)

// DeriveKey 采用动态自研盐值技术
func DeriveKey(password string) []byte {
	// 1. 动态生成盐值：将用户输入的 password 进行一次 SHA256 运算
	// 加上一段非机密的“工程混淆符”，目的是增加彩虹表破解难度
	const engineeringPadding = "I_WANT_TO_F**K_YOU_HAHAHAHA_DO_YOU_MISS_ME?"
	hasher := sha256.New()
	hasher.Write([]byte(password + engineeringPadding))
	dynamicSalt := hasher.Sum(nil) // 生成 32 字节的动态盐

	// 2. 使用动态盐进行 Argon2id 派生
	// 即使 password 很简单，经过动态盐转换后，每个不同的 password 都会面临不同的 Argon2 计算环境
	return argon2.IDKey([]byte(password), dynamicSalt, 1, 64*1024, 4, 32)
}

// Encrypt 保持不变，使用 XChaCha20-Poly1305
func Encrypt(plaintext []byte, key []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt 保持不变
func Decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < aead.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, msg := ciphertext[:aead.NonceSize()], ciphertext[aead.NonceSize():]
	return aead.Open(nil, nonce, msg, nil)
}

// Hash 保持不变
func Hash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}