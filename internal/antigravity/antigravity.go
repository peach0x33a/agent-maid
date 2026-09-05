// Package antigravity 封装 Google Antigravity 的凭据存储与账号切换。
//
// Antigravity 把 Google OAuth 凭据放在 gemini 生态目录下:
//
//	~/.gemini/oauth_creds.json      access/refresh/id_token 等(0600)
//	~/.gemini/google_accounts.json  active 账号邮箱与历史账号
//
// 与 ZCode 槽位相同的思路:两份文件按原文快照进槽位、原子写回实现切换,
// 不解密、不联网、不刷新 token。
package antigravity

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-maid/internal/atomicio"
)

// Creds 是 oauth_creds.json 的原样内容(保留未知字段)。
type Creds map[string]any

// Accounts 是 google_accounts.json 的原样内容。
type Accounts map[string]any

// GeminiDir 返回 gemini 生态数据目录(默认 ~/.gemini)。
func GeminiDir() string {
	if d := os.Getenv("ANTIGRAVITY_GEMINI_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".gemini")
}

// OAuthCredsPath 返回 oauth_creds.json 完整路径。
func OAuthCredsPath() string { return filepath.Join(GeminiDir(), "oauth_creds.json") }

// AccountsPath 返回 google_accounts.json 完整路径。
func AccountsPath() string { return filepath.Join(GeminiDir(), "google_accounts.json") }

// ReadCreds 读取 oauth_creds.json。
func ReadCreds() (Creds, error) {
	data, err := os.ReadFile(OAuthCredsPath())
	if err != nil {
		return nil, err
	}
	var creds Creds
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("oauth_creds.json 解析失败: %w", err)
	}
	return creds, nil
}

// WriteCreds 原子写回 oauth_creds.json,保持 0600。
func WriteCreds(creds Creds) error {
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteFile(OAuthCredsPath(), data, 0o600)
}

// ReadAccounts 读取 google_accounts.json;文件缺失时返回空表(nil, nil)。
func ReadAccounts() (Accounts, error) {
	data, err := os.ReadFile(AccountsPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var accounts Accounts
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, fmt.Errorf("google_accounts.json 解析失败: %w", err)
	}
	return accounts, nil
}

// WriteAccounts 原子写回 google_accounts.json(原文件 0644)。
func WriteAccounts(accounts Accounts) error {
	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteFile(AccountsPath(), data, 0o644)
}

// ActiveAccount 取 google_accounts.json 里的 active 邮箱。
func (a Accounts) ActiveAccount() string {
	if a == nil {
		return ""
	}
	s, _ := a["active"].(string)
	return s
}

// emailFromIDToken 解析 id_token(JWT)payload 里的 email,不验签。
// 切换只读本地声明的邮箱作展示标签。
func emailFromIDToken(idToken string) string {
	parts := strings.Split(strings.TrimSpace(idToken), ".")
	if len(parts) < 2 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		if raw, err = base64.URLEncoding.DecodeString(parts[1]); err != nil {
			return ""
		}
	}
	var payload struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	return payload.Email
}

// EqualCreds 判断两份凭据内容是否一致。
func EqualCreds(a, b Creds) bool {
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(ab) == string(bb)
}

// EqualAccounts 判断两份账号文件内容是否一致。
func EqualAccounts(a, b Accounts) bool {
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(ab) == string(bb)
}
