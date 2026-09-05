// Package zcode 封装对 ZCode 凭据存储的读取、解密与写回。
//
// ZCode 桌面端的凭据加密方案(从 app.asar 的 credentialService 逆向确认):
//
//	密文格式:  "enc:v1:" + b64url(IV) + "." + b64url(AuthTag) + "." + b64url(Ciphertext)
//	算法:      AES-256-GCM,IV 12 字节,AuthTag 16 字节
//	密钥:      SHA-256(secret)
//	secret:    环境变量 ZCODE_CREDENTIAL_SECRET,未设置时回退为
//	           "zcode-credential-fallback:{platform}:{homedir}:{username}"
//
// decrypt 遇到没有 enc:v1: 前缀的值会原样返回(兼容明文存储)。
package zcode

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	EncPrefix       = "enc:v1:"
	secretEnvVar    = "ZCODE_CREDENTIAL_SECRET"
	fallbackPrefix  = "zcode-credential-fallback"
	ErrDecryptShort = "密文格式非法"
)

// CredentialSecret 返回用于派生密钥的 secret 字符串,逻辑与 ZCode 一致。
func CredentialSecret() string {
	if s := os.Getenv(secretEnvVar); s != "" {
		return s
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "unknown"
	}
	username := "unknown"
	if u, err := user.Current(); err == nil && u.Username != "" {
		username = u.Username
	} else if u, err := user.LookupId(fmt.Sprintf("%d", os.Getuid())); err == nil && u.Username != "" {
		username = u.Username
	}
	return fmt.Sprintf("%s:%s:%s:%s", fallbackPrefix, runtime.GOOS, home, username)
}

// DeriveKey 返回 AES-256 密钥 = SHA-256(secret)。
func DeriveKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// KeyFingerprint 返回当前密钥的指纹(前 12 位 hex),用于槽位跨机器可解性检查。
func KeyFingerprint(key []byte) string {
	sum := sha256.Sum256(key)
	return fmt.Sprintf("%x", sum[:6])
}

// Decrypt 解密 enc:v1: 前缀的字符串;无前缀时原样返回。
func Decrypt(key []byte, value string) (string, error) {
	if !strings.HasPrefix(value, EncPrefix) {
		return value, nil
	}
	parts := strings.Split(value[len(EncPrefix):], ".")
	if len(parts) != 3 {
		return "", errors.New(ErrDecryptShort)
	}
	iv, err1 := base64.RawURLEncoding.DecodeString(parts[0])
	tag, err2 := base64.RawURLEncoding.DecodeString(parts[1])
	ct, err3 := base64.RawURLEncoding.DecodeString(parts[2])
	if err1 != nil || err2 != nil || err3 != nil || len(iv) != 12 || len(tag) != 16 {
		return "", errors.New(ErrDecryptShort)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	// Go 的 GCM.Seal/Open 约定:密文末尾 16 字节为 tag,与 Node 的 setAuthTag 对应
	plain, err := gcm.Open(nil, iv, append(ct, tag...), nil)
	if err != nil {
		return "", fmt.Errorf("密钥不匹配或密文已损坏: %w", err)
	}
	return string(plain), nil
}

// Encrypt 用与 ZCode 相同的格式加密字符串。
func Encrypt(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, iv, []byte(plaintext), nil)
	ct, tag := sealed[:len(sealed)-16], sealed[len(sealed)-16:]
	return EncPrefix +
		base64.RawURLEncoding.EncodeToString(iv) + "." +
		base64.RawURLEncoding.EncodeToString(tag) + "." +
		base64.RawURLEncoding.EncodeToString(ct), nil
}

// HomeDirOverride 仅供测试注入;为空时使用真实 home。
var HomeDirOverride string

func homeDir() string {
	if HomeDirOverride != "" {
		return HomeDirOverride
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// V2Dir 返回 ZCode 数据目录(默认 ~/.zcode/v2)。
// 可用环境变量 ZCODE_V2_DIR 覆盖(便于测试与便携场景)。
func V2Dir() string {
	if d := os.Getenv("ZCODE_V2_DIR"); d != "" {
		return d
	}
	return filepath.Join(homeDir(), ".zcode", "v2")
}

// CredentialsPath 返回 credentials.json 完整路径。
func CredentialsPath() string { return filepath.Join(V2Dir(), "credentials.json") }

// TelemetryStatePath 返回 telemetry-state.json 完整路径(内含 deviceMid)。
func TelemetryStatePath() string { return filepath.Join(V2Dir(), "telemetry-state.json") }
