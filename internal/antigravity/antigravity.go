// Package antigravity 封装 Antigravity(agy CLI 与 Antigravity Desktop)的登录凭据与账号切换。
//
// 新版 agy CLI 与 Antigravity Desktop 共用同一份 OS Keychain/Keyring 凭据
// (go-keyring: service="gemini", account="antigravity"),内容为单个 JSON 对象:
//
//	{
//	  "token": {"access_token":"...", "token_type":"Bearer",
//	            "refresh_token":"...", "expiry":"2026-10-26T06:43:49Z",
//	            "expires_in":3599},
//	  "auth_method": "consumer",
//	  "id_token_email": "user@example.com"
//	}
//
// 旧版 agy CLI 曾将同一份 JSON 存放在 ~/.gemini/antigravity-cli/antigravity-oauth-token,
// 本包在读取时保留对该文件的向后兼容。
//
// 槽位保存的是该凭据的原文 JSON 快照,切换即原子写回——与 ZCode 槽位同一
// 思路:不解密、不联网、不刷新 token(过期后由 Antigravity 自己用 refresh_token 续期)。
package antigravity

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zalando/go-keyring"

	"agent-maid/internal/atomicio"
)

// Token 是 Antigravity 凭据的原文 JSON 对象。保留原文而不是
// 结构体,是为了不丢掉后续版本可能新增的字段。
type Token = json.RawMessage

const (
	// tokenFileName 旧版 agy 的凭据文件名(登出时它会在前面加点并追加 .logged-out-<ts>)。
	tokenFileName = "antigravity-oauth-token"

	// keyringService / keyringAccount 是新版 agy CLI 与 Antigravity Desktop
	// 共用的 OS Keyring 凭据键名(由 zalando/go-keyring 写入)。
	keyringService = "gemini"
	keyringAccount = "antigravity"
)

type keyringStore interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

type osKeyring struct{}

func (osKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (osKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (osKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

var kr keyringStore = osKeyring{}

// useKeyring 判断当前是否应使用 macOS Keychain 读写凭据。
// 仅 macOS 上最新版 agy CLI 与 Antigravity Desktop 共用 Keychain;
// 其他平台继续使用 ~/.gemini/antigravity-cli/antigravity-oauth-token 文件。
// 当测试或用户显式覆盖了 ANTIGRAVITY_GEMINI_DIR / ANTIGRAVITY_CLI_DIR 时,
// 默认回退到文件模式,避免测试污染宿主机的真实 Keychain。
func useKeyring() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ANTIGRAVITY_KEYRING"))) {
	case "off", "false", "0", "file":
		return false
	case "on", "true", "1":
		return true
	}
	if runtime.GOOS != "darwin" {
		return false
	}
	return strings.TrimSpace(os.Getenv("ANTIGRAVITY_GEMINI_DIR")) == "" &&
		strings.TrimSpace(os.Getenv("ANTIGRAVITY_CLI_DIR")) == ""
}

// CredentialLocation 返回当前生效的凭据存储位置描述(用于展示与提示)。
func CredentialLocation() string {
	if useKeyring() {
		return fmt.Sprintf("Keychain (%s/%s)", keyringService, keyringAccount)
	}
	return TokenPath()
}

// GeminiDir 返回 gemini 生态数据目录(默认 ~/.gemini)。
func GeminiDir() string {
	if d := strings.TrimSpace(os.Getenv("ANTIGRAVITY_GEMINI_DIR")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".gemini")
}

// CLIDir 返回 agy 的数据目录(默认 <gemini>/antigravity-cli)。
func CLIDir() string {
	if d := strings.TrimSpace(os.Getenv("ANTIGRAVITY_CLI_DIR")); d != "" {
		return d
	}
	return filepath.Join(GeminiDir(), "antigravity-cli")
}

// TokenPath 返回 agy 登录凭据文件的完整路径。
func TokenPath() string { return filepath.Join(CLIDir(), tokenFileName) }

// parseTokenJSON 校验凭据字节是否为合法的 JSON 对象。
func parseTokenJSON(data []byte, source string) (Token, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("%s 为空", source)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("%s 不是合法 JSON", source)
	}
	var probe map[string]any
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("%s 不是 JSON 对象: %w", source, err)
	}
	return Token(data), nil
}

// compactTokenJSON 校验并压缩凭据为紧凑单行 JSON。
func compactTokenJSON(tok Token) ([]byte, error) {
	if len(bytes.TrimSpace(tok)) == 0 {
		return nil, errors.New("凭据内容为空")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, tok); err != nil {
		return nil, fmt.Errorf("凭据不是合法 JSON: %w", err)
	}
	var probe map[string]any
	if err := json.Unmarshal(compact.Bytes(), &probe); err != nil {
		return nil, fmt.Errorf("凭据不是 JSON 对象: %w", err)
	}
	return compact.Bytes(), nil
}

func readTokenFile() (Token, error) {
	data, err := os.ReadFile(TokenPath())
	if err != nil {
		return nil, err
	}
	return parseTokenJSON(data, TokenPath())
}

// ReadToken 读取 Antigravity 凭据。
// 优先读取 OS Keyring(新版 agy CLI 与 Antigravity Desktop 共用),
// 若 Keyring 未找到则回退到旧版凭据文件;都不存在时返回 os.ErrNotExist。
func ReadToken() (Token, error) {
	if useKeyring() {
		raw, err := kr.Get(keyringService, keyringAccount)
		if err == nil {
			return parseTokenJSON([]byte(raw), CredentialLocation())
		}
		if tok, fileErr := readTokenFile(); fileErr == nil {
			return tok, nil
		}
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("读取 %s 失败: %w", CredentialLocation(), err)
	}
	return readTokenFile()
}

// WriteToken 写回凭据。在 Keyring 模式下写入 OS Keyring(若旧版凭据文件
// 已存在则同步更新,避免版本混用时读到旧凭据);文件模式下原子写回 0600 文件。
func WriteToken(tok Token) error {
	compact, err := compactTokenJSON(tok)
	if err != nil {
		return err
	}
	if useKeyring() {
		if err := kr.Set(keyringService, keyringAccount, string(compact)); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", CredentialLocation(), err)
		}
		if _, err := os.Stat(TokenPath()); err == nil {
			if err := atomicio.WriteFile(TokenPath(), compact, 0o600); err != nil {
				return err
			}
		}
		return nil
	}
	return atomicio.WriteFile(TokenPath(), compact, 0o600)
}

// ClearToken 让 Antigravity 回到未登录态:清空 OS Keyring 凭据,并把旧版
// 凭据文件(若存在)改名成 .antigravity-oauth-token.logged-out-<时间> 保留。
func ClearToken() error {
	if useKeyring() {
		if err := kr.Delete(keyringService, keyringAccount); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("清除 %s 失败: %w", CredentialLocation(), err)
		}
	}
	path := TokenPath()
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	// 备份名与 agy 自身一致:.antigravity-oauth-token.logged-out-Y1026T041635Z
	dst := filepath.Join(filepath.Dir(path),
		fmt.Sprintf(".%s.logged-out-%s", tokenFileName, time.Now().Format("Y0102T150405Z")))
	if _, err := os.Stat(dst); err == nil {
		// 同一秒内重复登出:加纳秒区分,避免互相覆盖
		dst = filepath.Join(filepath.Dir(path),
			fmt.Sprintf(".%s.logged-out-%d", tokenFileName, time.Now().UnixNano()))
	}
	return os.Rename(path, dst)
}

// tokenDoc 解析整份凭据。
func tokenDoc(tok Token) map[string]any {
	if len(tok) == 0 {
		return nil
	}
	var doc map[string]any
	if json.Unmarshal(tok, &doc) != nil {
		return nil
	}
	return doc
}

// tokenInner 取嵌套的 token 对象。
func tokenInner(doc map[string]any) map[string]any {
	inner, _ := doc["token"].(map[string]any)
	return inner
}

// TokenEmail 取凭据里的账号邮箱:优先顶层 id_token_email,其次 token 内的
// id_token JWT(不验签,只读声明),再其次 token.id_token_email。
func TokenEmail(tok Token) string {
	doc := tokenDoc(tok)
	if doc == nil {
		return ""
	}
	if email, _ := doc["id_token_email"].(string); strings.TrimSpace(email) != "" {
		return strings.TrimSpace(email)
	}
	inner := tokenInner(doc)
	if inner == nil {
		return ""
	}
	if email := emailFromIDToken(str(inner["id_token"])); email != "" {
		return email
	}
	if email, _ := inner["id_token_email"].(string); strings.TrimSpace(email) != "" {
		return strings.TrimSpace(email)
	}
	return ""
}

// TokenAuthMethod 取认证方式(consumer / enterprise 等),用于展示。
func TokenAuthMethod(tok Token) string {
	doc := tokenDoc(tok)
	if doc == nil {
		return ""
	}
	method, _ := doc["auth_method"].(string)
	return strings.TrimSpace(method)
}

// TokenState 槽位凭据的时效状态。
type TokenState struct {
	ExpiresAt   *time.Time // token.expiry;无法解析时为 nil
	Refreshable bool       // 有 refresh_token:过期后 agy 可以自动续期
}

// Expired 报告凭据在 now 之前是否已过期。
func (s TokenState) Expired(now time.Time) bool {
	return s.ExpiresAt != nil && now.After(*s.ExpiresAt)
}

// TokenStateOf 解析凭据的时效信息。
func TokenStateOf(tok Token) TokenState {
	inner := tokenInner(tokenDoc(tok))
	if inner == nil {
		return TokenState{}
	}
	state := TokenState{Refreshable: str(inner["refresh_token"]) != ""}
	if t, ok := parseTime(str(inner["expiry"])); ok {
		state.ExpiresAt = &t
	}
	return state
}

// parseTime 解析 agy 写的时间串(RFC3339Nano,带纳秒)。
func parseTime(value string) (time.Time, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// EqualToken 判断两份凭据内容是否一致(按 JSON 语义比较,忽略空白与键序)。
func EqualToken(a, b Token) bool {
	if len(bytes.TrimSpace(a)) == 0 || len(bytes.TrimSpace(b)) == 0 {
		return false
	}
	var da, db any
	if json.Unmarshal(a, &da) != nil || json.Unmarshal(b, &db) != nil {
		return bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b))
	}
	return jsonEqual(da, db)
}

// jsonEqual 深度比较两个解码后的 JSON 值。
func jsonEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			other, ok := bv[k]
			if !ok || !jsonEqual(v, other) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
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

func str(v any) string {
	s, _ := v.(string)
	return s
}
