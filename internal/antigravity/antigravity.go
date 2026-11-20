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
// 本包在读取时保留对该文件的向后兼容;Linux 上还会兜底识别 agy 另一套
// Secret Service 条目(service="jetski-standalone-oauth-token")。
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

// useKeyring 判断当前是否应使用 OS Keychain/Keyring 读写凭据。
// macOS 与 Linux 上的新版 agy CLI / Antigravity Desktop 都会使用它;
// 其他平台继续使用 ~/.gemini/antigravity-cli/antigravity-oauth-token 文件。
// 当测试或用户显式覆盖了 ANTIGRAVITY_GEMINI_DIR / ANTIGRAVITY_CLI_DIR 时,
// 默认回退到文件模式,避免测试污染宿主机的真实钥匙串。
func useKeyring() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ANTIGRAVITY_KEYRING"))) {
	case "off", "false", "0", "file":
		return false
	case "on", "true", "1":
		return true
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return false
	}
	return strings.TrimSpace(os.Getenv("ANTIGRAVITY_GEMINI_DIR")) == "" &&
		strings.TrimSpace(os.Getenv("ANTIGRAVITY_CLI_DIR")) == ""
}

// keyringReadable 报告 OS 钥匙串当前是否可用(能读到条目或明确为空)。
// headless / 无 D-Bus 环境下 go-keyring 会返回"not activatable"之类的错误,
// 此时应像 agy 一样回落到文件,而不是把切换整个失败掉。
func keyringReadable() bool {
	if !useKeyring() {
		return false
	}
	_, err := kr.Get(keyringService, keyringAccount)
	return err == nil || errors.Is(err, keyring.ErrNotFound)
}

// keyringLocation 返回共用 Keychain/Keyring 条目的展示文本。
func keyringLocation() string {
	return fmt.Sprintf("系统钥匙串 (%s/%s)", keyringService, keyringAccount)
}

// CredentialLocation 返回凭据可能所在的位置(用于"未找到凭据"等提示)。
func CredentialLocation() string {
	if useKeyring() {
		return keyringLocation() + " 或 " + TokenPath()
	}
	return TokenPath()
}

// CredentialLocationFor 按实际来源返回凭据位置。
func CredentialLocationFor(source string) string {
	if source == SourceKeyring {
		return keyringLocation()
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

// 凭据来源。agy 在 Linux 上优先用系统钥匙串,读不到才回落到文件。
const (
	SourceKeyring = "keyring"
	SourceFile    = "file"
)

// ReadToken 读取 agy 当前生效的凭据(系统钥匙串优先,其次文件)。
// 文件缺失且钥匙串里也没有条目时返回 os.ErrNotExist 包装错误。
func ReadToken() (Token, error) {
	tok, _, err := ReadTokenWithSource()
	return tok, err
}

// ReadTokenWithSource 读取凭据并返回来源(SourceKeyring / SourceFile)。
// 顺序与 agy 一致:主钥匙串(gemini/antigravity)→ Linux 上的另一套
// Secret Service 条目 → 旧版文件;都不存在时返回 os.ErrNotExist。
func ReadTokenWithSource() (Token, string, error) {
	if useKeyring() {
		raw, keyErr := kr.Get(keyringService, keyringAccount)
		var parseErr error
		if keyErr == nil {
			tok, err := parseTokenJSON([]byte(raw), CredentialLocation())
			if err == nil {
				return tok, SourceKeyring, nil
			}
			parseErr = err
		}
		// Linux:agy 的另一些版本把同一份凭据放在
		// service=jetski-standalone-oauth-token 下。
		if tok, ok := readKeyringToken(); ok {
			return tok, SourceKeyring, nil
		}
		if tok, fileErr := readTokenFile(); fileErr == nil {
			return tok, SourceFile, nil
		}
		if parseErr != nil {
			return nil, "", parseErr
		}
		if keyErr == nil || errors.Is(keyErr, keyring.ErrNotFound) {
			return nil, "", os.ErrNotExist
		}
		return nil, "", fmt.Errorf("读取 %s 失败: %w", CredentialLocation(), keyErr)
	}
	tok, err := readTokenFile()
	if err != nil {
		return nil, "", err
	}
	return tok, SourceFile, nil
}

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

// readTokenFile 读取旧版 agy 的文件凭据。
func readTokenFile() (Token, error) {
	data, err := os.ReadFile(TokenPath())
	if err != nil {
		return nil, err
	}
	return parseTokenJSON(data, TokenPath())
}

// WriteToken 写回凭据。Keyring 模式下写入共用 Keychain/Keyring 条目
// (Linux 上若存在 agy 的另一套 Secret Service 条目也同步更新),旧版凭据
// 文件已存在时一并更新;文件模式下原子写回 0600 文件。
func WriteToken(tok Token) error {
	compact, err := compactTokenJSON(tok)
	if err != nil {
		return err
	}
	if useKeyring() && keyringReadable() {
		if err := writeKeyringToken(Token(compact)); err != nil {
			return err
		}
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

// ClearToken 让 Antigravity 回到未登录态:清空 OS Keyring 凭据(Linux 上
// 连同另一套 Secret Service 条目),并把旧版凭据文件(若存在)改名成
// .antigravity-oauth-token.logged-out-<时间> 保留,误登出还能找回。
func ClearToken() error {
	if useKeyring() && keyringReadable() {
		if err := kr.Delete(keyringService, keyringAccount); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("清除 %s 失败: %w", CredentialLocation(), err)
		}
		if err := deleteKeyringToken(); err != nil {
			return err
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

// TokenIdentity 返回账号身份键:优先邮箱(agy 写入的 id_token_email),
// 没有邮箱时退回 refresh_token;都没有则返回空,只能按原文比较。
// 用于识别"agy 自己刷新过 access_token"后的同一账号。
func TokenIdentity(tok Token) string {
	if email := TokenEmail(tok); email != "" {
		return "email:" + strings.ToLower(email)
	}
	inner := tokenInner(tokenDoc(tok))
	if inner == nil {
		return ""
	}
	if rt := strings.TrimSpace(str(inner["refresh_token"])); rt != "" {
		return "refresh:" + rt
	}
	return ""
}

// SameAccount 判断两份凭据是否属于同一个账号。agy 会每小时用
// refresh_token 续期并重写凭据(access_token/expiry 变化),所以"正在使用"
// 不能只按原文比较;身份相同时还要看认证方式是否冲突(consumer/enterprise)。
func SameAccount(a, b Token) bool {
	if EqualToken(a, b) {
		return true
	}
	ida, idb := TokenIdentity(a), TokenIdentity(b)
	if ida == "" || ida != idb {
		return false
	}
	ma, mb := TokenAuthMethod(a), TokenAuthMethod(b)
	return ma == "" || mb == "" || ma == mb
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
