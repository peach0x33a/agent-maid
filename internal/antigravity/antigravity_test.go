package antigravity

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupIsolated 把 gemini 数据目录、agy 数据目录与槽位目录都指到临时目录。
func setupIsolated(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("ANTIGRAVITY_GEMINI_DIR", filepath.Join(base, "gemini"))
	t.Setenv("ANTIGRAVITY_CLI_DIR", "")
	t.Setenv("ANTIGRAVITY_STORE_DIR", filepath.Join(base, "slots"))
	return base
}

// writeTokenFile 按 agy 的真实格式写入凭据文件:紧凑单行、0600、无结尾换行。
func writeTokenFile(t *testing.T, doc map[string]any) {
	t.Helper()
	if err := os.MkdirAll(CLIDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(TokenPath(), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fakeIDToken(email string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"` + email + `"}`))
	return header + "." + payload + ".sig"
}

// tokenDocFor 造一份与 agy 实际写盘结构一致的凭据。
func tokenDocFor(access, refresh, email string, expiry time.Time) map[string]any {
	return map[string]any{
		"token": map[string]any{
			"access_token":  access,
			"token_type":    "Bearer",
			"refresh_token": refresh,
			"expiry":        expiry.UTC().Format(time.RFC3339Nano),
			"expires_in":    3599,
		},
		"auth_method":    "consumer",
		"id_token_email": email,
	}
}

func TestTokenPathLayout(t *testing.T) {
	setupIsolated(t)
	want := filepath.Join(GeminiDir(), "antigravity-cli", "antigravity-oauth-token")
	if got := TokenPath(); got != want {
		t.Fatalf("TokenPath = %q, 期望 %q", got, want)
	}
	if !strings.HasSuffix(TokenPath(), "antigravity-cli/antigravity-oauth-token") {
		t.Fatalf("凭据文件名不对: %q", TokenPath())
	}
}

func TestEmailFromIDToken(t *testing.T) {
	if got := emailFromIDToken(fakeIDToken("a@b.com")); got != "a@b.com" {
		t.Fatalf("emailFromIDToken = %q, 期望 a@b.com", got)
	}
	if emailFromIDToken("not-a-jwt") != "" {
		t.Fatal("非法 JWT 应返回空")
	}
}

// 邮箱优先取顶层 id_token_email(agy 自己写入的字段),它缺失时再退回 JWT。
func TestTokenEmailPreference(t *testing.T) {
	withTop := Token(`{"token":{"access_token":"a"},"id_token_email":"top@example.com"}`)
	if got := TokenEmail(withTop); got != "top@example.com" {
		t.Fatalf("应优先取 id_token_email,得到 %q", got)
	}
	fromJWT := Token(`{"token":{"access_token":"a","id_token":"` + fakeIDToken("jwt@example.com") + `"}}`)
	if got := TokenEmail(fromJWT); got != "jwt@example.com" {
		t.Fatalf("应回退到 id_token JWT,得到 %q", got)
	}
	if got := TokenEmail(Token(`{"token":{"access_token":"a"}}`)); got != "" {
		t.Fatalf("没有邮箱信息时应为空,得到 %q", got)
	}
}

func TestTokenStateOf(t *testing.T) {
	exp := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	st := TokenStateOf(Token(`{"token":{"refresh_token":"rt","expiry":"` + exp.Format(time.RFC3339Nano) + `"}}`))
	if !st.Refreshable {
		t.Fatal("有 refresh_token 应标记为可续期")
	}
	if st.ExpiresAt == nil || !st.ExpiresAt.Equal(exp) {
		t.Fatalf("ExpiresAt = %v, 期望 %v", st.ExpiresAt, exp)
	}
	if st.Expired(exp.Add(-time.Minute)) {
		t.Fatal("过期前不应报告为过期")
	}
	if !st.Expired(exp.Add(time.Minute)) {
		t.Fatal("过期后应报告为过期")
	}
}

// 保存 → 换账号 → 切换,凭据文件必须还原成 A 的原文(紧凑格式、0600)。
func TestSaveAndApplyRoundtrip(t *testing.T) {
	setupIsolated(t)
	svc := New()
	expA := time.Now().Add(time.Hour).Round(time.Second)

	writeTokenFile(t, tokenDocFor("access-A", "refresh-A", "a@example.com", expA))
	saved, err := svc.SaveCurrent("work", false)
	if err != nil {
		t.Fatalf("SaveCurrent: %v", err)
	}
	if saved.Email != "a@example.com" || saved.AuthMode != "consumer" {
		t.Fatalf("槽位标签不对: %+v", saved)
	}

	cur := svc.Current()
	if !cur.Exists || cur.Email != "a@example.com" || cur.AuthMode != "consumer" {
		t.Fatalf("Current = %+v", cur)
	}

	// agy 里换成另一个账号
	writeTokenFile(t, tokenDocFor("access-B", "refresh-B", "b@example.com", expA))

	backup, err := svc.Apply("work")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if backup == "" {
		t.Fatal("未保存的当前账号应先自动备份")
	}

	tok, err := ReadToken()
	if err != nil {
		t.Fatal(err)
	}
	if got := TokenEmail(tok); got != "a@example.com" {
		t.Fatalf("切换后邮箱 = %q, 期望 a@example.com", got)
	}
	if !strings.Contains(string(tok), "access-A") {
		t.Fatalf("切换后凭据内容不对: %s", tok)
	}

	// 写回格式必须与 agy 一致:紧凑 JSON、无缩进、无结尾换行、0600
	raw, err := os.ReadFile(TokenPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "\n") {
		t.Fatalf("写回的凭据不应包含换行(agy 是紧凑单行): %q", raw)
	}
	fi, err := os.Stat(TokenPath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("凭据文件权限 = %04o, 期望 0600", perm)
	}

	// 自动备份槽位应该存在
	if _, err := svc.Store.Get(backup); err != nil {
		t.Fatalf("自动备份槽位 %q 读不到: %v", backup, err)
	}
}

func TestApplyNoopWhenEqual(t *testing.T) {
	setupIsolated(t)
	svc := New()
	writeTokenFile(t, tokenDocFor("tok-A", "rt-A", "a@example.com", time.Now().Add(time.Hour)))
	if _, err := svc.SaveCurrent("same", false); err != nil {
		t.Fatal(err)
	}
	backup, err := svc.Apply("same")
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" {
		t.Fatalf("内容一致时不应触发自动备份,得到 %q", backup)
	}
}

// 未登录时保存要给出可执行的提示,而不是底层文件错误。
func TestSaveCurrentWithoutLogin(t *testing.T) {
	setupIsolated(t)
	_, err := New().SaveCurrent("work", false)
	if err == nil {
		t.Fatal("未登录时保存应报错")
	}
	if !strings.Contains(err.Error(), "请先在 agy 里登录") {
		t.Fatalf("错误提示不够明确: %v", err)
	}
}

// 登出改名而不是删除:与 agy 自身行为一致,误登出还能找回。
func TestClearTokenRenamesLikeAgy(t *testing.T) {
	setupIsolated(t)
	writeTokenFile(t, tokenDocFor("a", "r", "a@example.com", time.Now().Add(time.Hour)))
	if err := ClearToken(); err != nil {
		t.Fatalf("ClearToken: %v", err)
	}
	if _, err := os.Stat(TokenPath()); !os.IsNotExist(err) {
		t.Fatal("登出后原凭据文件应不存在")
	}
	entries, err := os.ReadDir(CLIDir())
	if err != nil {
		t.Fatal(err)
	}
	found := ""
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".antigravity-oauth-token.logged-out-") {
			found = e.Name()
		}
	}
	if found == "" {
		t.Fatalf("登出应留下 .logged-out-* 备份,实际目录: %v", entries)
	}
	// 命名与 agy 自身一致:Y + 年月日 + T + 时分秒 + Z
	suffix := strings.TrimPrefix(found, ".antigravity-oauth-token.logged-out-")
	if len(suffix) != len("Y1026T041635Z") || suffix[0] != 'Y' || !strings.HasSuffix(suffix, "Z") {
		t.Fatalf("登出备份名 %q 与 agy 的 Y<MMDD>T<HHMMSS>Z 格式不一致", found)
	}
	// 再登出一次不应报错(文件已不存在)
	if err := ClearToken(); err != nil {
		t.Fatalf("重复登出应静默: %v", err)
	}
}

func TestSlotNameSanitized(t *testing.T) {
	clean, err := SanitizeName("../evil")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(clean, "/") || strings.Contains(clean, "..") {
		t.Fatalf("路径穿越名应被清洗,得到 %q", clean)
	}
	if clean, err = SanitizeName("  work 1 "); err != nil || clean != "work 1" {
		t.Fatalf("SanitizeName = %q, %v", clean, err)
	}
	if _, err := SanitizeName("   "); err == nil {
		t.Fatal("空名应报错")
	}
}

// 槽位文件里保存的是 agy 凭据原文,且不落盘派生状态。
func TestSlotStoresRawToken(t *testing.T) {
	setupIsolated(t)
	svc := New()
	writeTokenFile(t, tokenDocFor("access-A", "refresh-A", "a@example.com", time.Now().Add(time.Hour)))
	if _, err := svc.SaveCurrent("work", false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(DefaultDir(), "work.json"))
	if err != nil {
		t.Fatal(err)
	}
	var slot map[string]any
	if err := json.Unmarshal(raw, &slot); err != nil {
		t.Fatal(err)
	}
	if slot["email"] != "a@example.com" {
		t.Fatalf("槽位 email = %v", slot["email"])
	}
	if _, ok := slot["token"]; !ok {
		t.Fatalf("槽位应包含 token 原文: %s", raw)
	}
	if _, ok := slot["authMethod"]; !ok {
		t.Fatalf("槽位应记录 authMethod: %s", raw)
	}
}
