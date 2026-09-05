package zcode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Credentials 是 credentials.json 的原样内容(key → 密文或明文值)。
// 保存/恢复一律按原文搬运,不需要也不必要重加密。
type Credentials map[string]string

const (
	KeyActiveProvider = "oauth:active_provider"
	KeyUserPrefix     = "oauth:"
	KeyUserSuffix     = ":user_info"
)

// ReadCredentials 读取 credentials.json。
func ReadCredentials() (Credentials, error) {
	data, err := os.ReadFile(CredentialsPath())
	if err != nil {
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("credentials.json 解析失败: %w", err)
	}
	return creds, nil
}

// WriteCredentials 原子写回 credentials.json,保持 0600 权限。
func WriteCredentials(creds Credentials) error {
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(CredentialsPath(), data, 0o600)
}

// ClearCredentials 删除本地 credentials.json(仅本地登出,
// 不吊销服务端令牌);文件不存在时静默。
func ClearCredentials() error {
	err := os.Remove(CredentialsPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// CredentialsExists 判断 credentials.json 是否存在。
func CredentialsExists() bool {
	_, err := os.Stat(CredentialsPath())
	return err == nil
}

// AccountMeta 从凭据中解出的账号摘要(用于槽位标签展示)。
type AccountMeta struct {
	Provider    string `json:"provider,omitempty"`
	UserID      string `json:"userId,omitempty"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

// ExtractMeta 在内存中解密凭据,提取账号摘要。失败字段留空,不视为致命错误。
func (c Credentials) ExtractMeta(key []byte) AccountMeta {
	meta := AccountMeta{}
	if v, ok := c[KeyActiveProvider]; ok {
		if s, err := Decrypt(key, v); err == nil {
			meta.Provider = s
		}
	}
	// user_info 的 key 形如 oauth:bigmodel:user_info;以 active_provider 对应为准
	for k, v := range c {
		if !strings.HasPrefix(k, KeyUserPrefix) || !strings.HasSuffix(k, KeyUserSuffix) {
			continue
		}
		if meta.Provider != "" && !strings.Contains(k, ":"+meta.Provider+":") {
			continue
		}
		plain, err := Decrypt(key, v)
		if err != nil {
			continue
		}
		var info struct {
			ID          string `json:"id"`
			Username    string `json:"username"`
			DisplayName string `json:"displayName"`
		}
		if json.Unmarshal([]byte(plain), &info) == nil {
			meta.UserID = info.ID
			meta.Username = info.Username
			meta.DisplayName = info.DisplayName
			break
		}
	}
	return meta
}

// MaskedValue 返回适合展示的值:密文与长明文打码,只留尾 4 位。
func MaskedValue(k, v string) string {
	if len(v) <= 12 {
		return v
	}
	return "••••••••" + v[len(v)-4:] + fmt.Sprintf("  (%s, %d 字符)", valueKind(v), len(v))
}

func valueKind(v string) string {
	switch {
	case strings.HasPrefix(v, EncPrefix):
		return "enc:v1 密文"
	case strings.HasPrefix(v, "eyJ"):
		return "JWT"
	default:
		return "明文"
	}
}

// DeviceMid 从 telemetry-state.json 读取机器码。
func DeviceMid() (string, error) {
	data, err := os.ReadFile(TelemetryStatePath())
	if err != nil {
		return "", err
	}
	var state struct {
		DeviceMid string `json:"deviceMid"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return "", err
	}
	if state.DeviceMid == "" {
		return "", errors.New("telemetry-state.json 中没有 deviceMid")
	}
	return state.DeviceMid, nil
}

// atomicWrite 临时文件 + rename 的原子写入。
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".zacc-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // rename 成功后 Remove 返回 ENOENT,无害
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
