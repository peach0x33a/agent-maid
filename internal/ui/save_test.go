package ui

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"agent-maid/internal/gptauth"
	"agent-maid/internal/gptstore"
)

// writeAuthJSON 在测试用的 CODEX_HOME 里写入一份可解析的 auth.json。
func writeAuthJSON(t *testing.T, accountID, email string) {
	t.Helper()
	home, err := gptauth.ResolveHome("")
	if err != nil {
		t.Fatal(err)
	}
	payload := func(v map[string]any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	// 形如 id_token: <header>.<claims>.<sig>,邮箱在 claims 里
	idToken := payload(map[string]any{"alg": "none", "typ": "JWT"}) + "." + payload(map[string]any{
		"email": email,
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": accountID,
			"chatgpt_plan_type":  "plus",
			"user_id":            "u-1",
		},
	}) + ".sig"
	data, err := gptauth.BuildAuthFile(gptauth.Tokens{
		IDToken:      idToken,
		AccessToken:  idToken,
		RefreshToken: "rt-1",
		AccountID:    accountID,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gptauth.WriteAuthFile(home, data); err != nil {
		t.Fatal(err)
	}
}

// 保存当前凭据时,如果账号已存在且用户没输入新名字,必须保留用户改过的
// 显示名(而不是覆盖回邮箱)。
func TestSaveCurrentKeepsCustomLabel(t *testing.T) {
	setupIsolated(t)
	writeAuthJSON(t, "acc-1", "user@example.com")

	m := NewChatgpt()
	if _, err := m.saveCurrent("我改过的名字"); err != nil {
		t.Fatalf("首次保存失败: %v", err)
	}
	// 再保存一次但不给名字(登出时走的就是这条路径)
	if _, err := m.saveCurrent(""); err != nil {
		t.Fatalf("再次保存失败: %v", err)
	}

	st, err := gptstore.Load(gptstore.DefaultPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Data.Accounts) != 1 {
		t.Fatalf("应只有一个账号,实际 %d", len(st.Data.Accounts))
	}
	if got := st.Data.Accounts[0].Label; got != "我改过的名字" {
		t.Fatalf("显示名被覆盖为 %q,应为 我改过的名字", got)
	}

	// 显式给名字时仍然要更新
	if _, err := m.saveCurrent("另一个名字"); err != nil {
		t.Fatal(err)
	}
	st, _ = gptstore.Load(gptstore.DefaultPath())
	if got := st.Data.Accounts[0].Label; got != "另一个名字" {
		t.Fatalf("显式重命名未生效,得到 %q", got)
	}
}
