package antigravity

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupIsolated 把 gemini 数据目录与槽位目录都指到临时目录。
func setupIsolated(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("ANTIGRAVITY_GEMINI_DIR", filepath.Join(base, "gemini"))
	t.Setenv("ANTIGRAVITY_STORE_DIR", filepath.Join(base, "slots"))
	return base
}

func writeAuthFiles(t *testing.T, creds Creds, accounts Accounts) {
	t.Helper()
	if err := os.MkdirAll(GeminiDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	credData, _ := json.Marshal(creds)
	if err := os.WriteFile(OAuthCredsPath(), credData, 0o600); err != nil {
		t.Fatal(err)
	}
	if accounts != nil {
		accData, _ := json.Marshal(accounts)
		if err := os.WriteFile(AccountsPath(), accData, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func fakeIDToken(email string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"` + email + `"}`))
	return header + "." + payload + ".sig"
}

func TestEmailFromIDToken(t *testing.T) {
	got := emailFromIDToken(fakeIDToken("a@b.com"))
	if got != "a@b.com" {
		t.Fatalf("emailFromIDToken = %q, 期望 a@b.com", got)
	}
	if emailFromIDToken("not-a-jwt") != "" {
		t.Fatal("非法 JWT 应返回空")
	}
}

func TestSaveAndApplyRoundtrip(t *testing.T) {
	setupIsolated(t)
	svc := New()

	writeAuthFiles(t, Creds{"access_token": "tok-A", "refresh_token": "rt-A", "id_token": fakeIDToken("a@b.com")},
		Accounts{"active": "a@b.com"})

	if _, err := svc.SaveCurrent("work", false); err != nil {
		t.Fatalf("SaveCurrent: %v", err)
	}
	cur := svc.Current()
	if !cur.Exists || cur.Email != "a@b.com" {
		t.Fatalf("Current = %+v", cur)
	}

	// 换成另一个账号
	writeAuthFiles(t, Creds{"access_token": "tok-B", "refresh_token": "rt-B", "id_token": fakeIDToken("b@b.com")},
		Accounts{"active": "b@b.com"})

	backup, err := svc.Apply("work")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if backup == "" {
		t.Fatal("未覆盖的当前账号应先自动备份")
	}
	creds, err := ReadCreds()
	if err != nil {
		t.Fatal(err)
	}
	if creds["access_token"] != "tok-A" {
		t.Fatalf("切换后 access_token = %v, 期望 tok-A", creds["access_token"])
	}
	accounts, _ := ReadAccounts()
	if accounts.ActiveAccount() != "a@b.com" {
		t.Fatalf("切换后 active = %q, 期望 a@b.com", accounts.ActiveAccount())
	}
}

func TestApplyNoopWhenEqual(t *testing.T) {
	setupIsolated(t)
	svc := New()
	writeAuthFiles(t, Creds{"access_token": "tok-A", "id_token": fakeIDToken("a@b.com")}, Accounts{"active": "a@b.com"})
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
