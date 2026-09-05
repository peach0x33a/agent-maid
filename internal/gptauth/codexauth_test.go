package gptauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testTokens = Tokens{
	IDToken:      "id-token",
	AccessToken:  "access-token",
	RefreshToken: "refresh-token",
	AccountID:    "acc-1",
}

func TestBuildAuthFileNativeShapeAndOrder(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	raw, err := BuildAuthFile(testTokens, now)
	if err != nil {
		t.Fatalf("BuildAuthFile: %v", err)
	}
	text := string(raw)
	wantOrder := `"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"id_token":"id-token","access_token":"access-token","refresh_token":"refresh-token","account_id":"acc-1"},"last_refresh":"2026-08-30T12:00:00Z"`
	if !strings.Contains(strings.Join(strings.Fields(text), ""), strings.Join(strings.Fields(wantOrder), "")) {
		t.Fatalf("auth.json shape/order mismatch:\n%s", text)
	}
	// Pretty-printed with two spaces and a trailing newline.
	if !strings.HasPrefix(text, "{\n  \"auth_mode\": \"chatgpt\",") || !strings.HasSuffix(text, "}\n") {
		t.Errorf("unexpected formatting:\n%s", text)
	}
}

func TestMergeAuthFilePreservesUnknownKeysAndOrder(t *testing.T) {
	existing := []byte(`{
  "auth_mode": "apikey",
  "OPENAI_API_KEY": "sk-existing",
  "tokens": {"id_token": "old"},
  "last_refresh": "2020-01-01T00:00:00Z",
  "custom_note": "keep me"
}`)
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	merged, err := MergeAuthFile(existing, testTokens, now)
	if err != nil {
		t.Fatalf("MergeAuthFile: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(merged, &doc); err != nil {
		t.Fatalf("merged is not JSON: %v", err)
	}
	if doc["auth_mode"] != "chatgpt" {
		t.Errorf("auth_mode = %v", doc["auth_mode"])
	}
	if doc["OPENAI_API_KEY"] != nil {
		t.Errorf("OPENAI_API_KEY must be cleared, got %v", doc["OPENAI_API_KEY"])
	}
	if doc["custom_note"] != "keep me" {
		t.Errorf("unknown key lost: %v", doc["custom_note"])
	}
	tokens, _ := doc["tokens"].(map[string]any)
	if tokens["access_token"] != "access-token" || tokens["account_id"] != "acc-1" {
		t.Errorf("tokens not replaced: %v", tokens)
	}
	if doc["last_refresh"] != "2026-08-30T12:00:00Z" {
		t.Errorf("last_refresh = %v", doc["last_refresh"])
	}
	// Unknown key stays behind the managed keys, order preserved.
	order := []string{"auth_mode", "OPENAI_API_KEY", "tokens", "last_refresh", "custom_note"}
	pos := 0
	for _, key := range order {
		idx := strings.Index(string(merged), "\""+key+"\"")
		if idx < pos {
			t.Fatalf("key %q out of order in:\n%s", key, merged)
		}
		pos = idx
	}
}

func TestMergeAuthFileInvalidExistingStartsFresh(t *testing.T) {
	merged, err := MergeAuthFile([]byte("not json"), testTokens, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("MergeAuthFile: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(merged, &doc); err != nil {
		t.Fatalf("merged is not JSON: %v", err)
	}
}

func TestWriteAuthFileBackupAndPermissions(t *testing.T) {
	home := t.TempDir()
	first, err := BuildAuthFile(Tokens{IDToken: "a", AccessToken: "a", RefreshToken: "a", AccountID: "a"}, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	backup, changed, err := WriteAuthFile(home, first)
	if err != nil || !changed || backup != "" {
		t.Fatalf("fresh write: backup=%q changed=%v err=%v", backup, changed, err)
	}

	second, err := BuildAuthFile(testTokens, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	backup, changed, err = WriteAuthFile(home, second)
	if err != nil || !changed {
		t.Fatalf("second write: changed=%v err=%v", changed, err)
	}
	if backup == "" {
		t.Fatal("expected a backup of the replaced file")
	}
	saved, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("backup unreadable: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(saved), bytes.TrimSpace(first)) {
		t.Error("backup does not contain the previous credentials")
	}
	info, err := os.Stat(AuthFilePath(home))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("auth.json perm = %o, want 600", perm)
	}
	backupInfo, err := os.Stat(backup)
	if err != nil {
		t.Fatal(err)
	}
	if perm := backupInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("backup perm = %o, want 600", perm)
	}

	// Writing identical bytes is a no-op (no backup churn).
	_, changed, err = WriteAuthFile(home, second)
	if err != nil || changed {
		t.Fatalf("idempotent write: changed=%v err=%v", changed, err)
	}
}

func TestWriteAuthFilePrunesOldBackups(t *testing.T) {
	home := t.TempDir()
	seed, err := BuildAuthFile(Tokens{IDToken: "seed", AccessToken: "seed", RefreshToken: "seed", AccountID: "seed"}, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteAuthFile(home, seed); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 14; i++ {
		name := filepath.Join(home, fmt.Sprintf("auth.json.bak-20260830-0000%02d", i))
		if err := os.WriteFile(name, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := BuildAuthFile(testTokens, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, changed, err := WriteAuthFile(home, data); err != nil || !changed {
		t.Fatalf("write: changed=%v err=%v", changed, err)
	}
	matches, _ := filepath.Glob(filepath.Join(home, "auth.json.bak-*"))
	if len(matches) != backupKeep {
		t.Errorf("backups after prune = %d, want %d", len(matches), backupKeep)
	}
}

func TestResolveHome(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	home, err := ResolveHome("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(home, ".codex") {
		t.Errorf("default home = %q", home)
	}
	t.Setenv("CODEX_HOME", "/custom/codex")
	home, err = ResolveHome("")
	if err != nil {
		t.Fatal(err)
	}
	if home != "/custom/codex" {
		t.Errorf("CODEX_HOME ignored: %q", home)
	}
	home, err = ResolveHome("/flag/dir")
	if err != nil {
		t.Fatal(err)
	}
	if home != "/flag/dir" {
		t.Errorf("flag ignored: %q", home)
	}
}

func TestReadCurrent(t *testing.T) {
	home := t.TempDir()
	current, err := ReadCurrent(home)
	if err != nil || current != nil {
		t.Fatalf("missing file: current=%v err=%v", current, err)
	}
	data, err := BuildAuthFile(testTokens, time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteAuthFile(home, data); err != nil {
		t.Fatal(err)
	}
	current, err = ReadCurrent(home)
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	if current.AuthMode != "chatgpt" || current.AccountID != "acc-1" || !current.HasTokens {
		t.Errorf("current = %+v", current)
	}
	if current.LastRefreshTime == nil {
		t.Error("last_refresh not parsed")
	}
}

func TestReadCurrentBrokenFile(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(AuthFilePath(home), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCurrent(home); err == nil {
		t.Fatal("expected an error for a broken auth.json")
	}
}
