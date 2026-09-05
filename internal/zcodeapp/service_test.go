package zcodeapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"agent-maid/internal/zcode"
	"agent-maid/internal/zcodestore"
)

// setupTemp 用临时目录隔离 ZCode 数据目录与槽位存储。
func setupTemp(t *testing.T) *Service {
	t.Helper()
	v2 := t.TempDir()
	slots := t.TempDir()
	t.Setenv("ZCODE_V2_DIR", v2)
	t.Setenv("ZCODE_ACC_SWITCH_DIR", slots)
	return New()
}

// writeFakeCredentials 生成一份结构上仿真的 credentials.json(user_info 用真实加密)。
func writeFakeCredentials(t *testing.T, provider, username string) {
	t.Helper()
	key := zcode.DeriveKey(zcode.CredentialSecret())
	enc := func(s string) string {
		v, err := zcode.Encrypt(key, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	creds := zcode.Credentials{
		"oauth:" + provider + ":access_token": enc("fake-access-token-jwt"),
		"zcodejwttoken":                       enc("fake-agent-jwt"),
		"oauth:" + provider + ":user_info":    enc(`{"id":"u-1","username":"` + username + `","displayName":"` + username + `-显示名"}`),
		"oauth:active_provider":               enc(provider),
	}
	if err := zcode.WriteCredentials(creds); err != nil {
		t.Fatal(err)
	}
}

func TestSaveAndApplyRoundTrip(t *testing.T) {
	svc := setupTemp(t)
	writeFakeCredentials(t, "bigmodel", "alice")

	// 保存槽位 work
	if _, err := svc.SaveCurrent("work", false); err != nil {
		t.Fatal(err)
	}
	slot, err := svc.Store.Get("work")
	if err != nil {
		t.Fatal(err)
	}
	if slot.Meta.Provider != "bigmodel" || slot.Meta.Username != "alice" {
		t.Fatalf("槽位元数据不符: %+v", slot.Meta)
	}

	// 换成 bob 的凭据,再保存槽位 home
	writeFakeCredentials(t, "bigmodel", "bob")
	if _, err := svc.SaveCurrent("home", false); err != nil {
		t.Fatal(err)
	}

	// 切回 work:当前是 bob(已被 home 覆盖)→ 不应触发自动备份
	backup, err := svc.Apply("work")
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" {
		t.Fatalf("bob 已被槽位覆盖,不应自动备份,却备份到 %q", backup)
	}
	cur := svc.Current()
	if cur.Meta.Username != "alice" {
		t.Fatalf("切换后应是 alice,得到 %+v", cur.Meta)
	}

	// 写入一份没有任何槽位覆盖的新凭据 → 切换时应自动备份
	writeFakeCredentials(t, "zai", "carol")
	backup, err = svc.Apply("work")
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("未覆盖的当前账号应被自动备份")
	}
	if !svc.Store.Has(backup) {
		t.Fatalf("自动备份槽位 %s 不存在", backup)
	}
	var backupSlot zcodestore.Slot
	data, _ := os.ReadFile(filepath.Join(os.Getenv("ZCODE_ACC_SWITCH_DIR"), "slots", backup+".json"))
	json.Unmarshal(data, &backupSlot)
	if backupSlot.Meta.Username != "carol" {
		t.Fatalf("备份内容不符: %+v", backupSlot.Meta)
	}

	// 切换后当前账号是 alice
	if cur := svc.Current(); cur.Meta.Username != "alice" {
		t.Fatalf("二次切换后应是 alice,得到 %+v", cur.Meta)
	}
}

func TestSaveOverwriteGuard(t *testing.T) {
	svc := setupTemp(t)
	writeFakeCredentials(t, "bigmodel", "alice")
	if _, err := svc.SaveCurrent("work", false); err != nil {
		t.Fatal(err)
	}
	// 不允许覆盖时,重名应报错
	if _, err := svc.SaveCurrent("work", false); err == nil {
		t.Fatal("重复保存应报错")
	}
	// 允许覆盖时成功
	if _, err := svc.SaveCurrent("work", true); err != nil {
		t.Fatal(err)
	}
}

func TestApplySameContentIsNoop(t *testing.T) {
	svc := setupTemp(t)
	writeFakeCredentials(t, "bigmodel", "alice")
	if _, err := svc.SaveCurrent("work", false); err != nil {
		t.Fatal(err)
	}
	backup, err := svc.Apply("work")
	if err != nil || backup != "" {
		t.Fatalf("同内容切换应为 noop: backup=%q err=%v", backup, err)
	}
}

func TestStoreNameSanitize(t *testing.T) {
	svc := setupTemp(t)
	writeFakeCredentials(t, "bigmodel", "alice")
	// 含路径穿越的名字被清洗为无害文件名,而不是写到 slots 目录之外
	slot, err := svc.SaveCurrent("../evil", false)
	if err != nil {
		t.Fatal(err)
	}
	if slot.Name != "___evil" {
		t.Fatalf("穿越名应被清洗,得到 %q", slot.Name)
	}
	// 确认没有写出 slots 目录之外的文件
	entries, _ := os.ReadDir(os.Getenv("ZCODE_ACC_SWITCH_DIR"))
	for _, e := range entries {
		if e.Name() != "slots" {
			t.Fatalf("存储目录中出现意外文件: %s", e.Name())
		}
	}
	// 含非法字符的名字被清洗,但仍可保存/取出
	slot2, err2 := svc.SaveCurrent("my:account*01", false)
	if err2 != nil {
		t.Fatal(err2)
	}
	if !svc.Store.Has(slot2.Name) {
		t.Fatal("清洗后的槽位应存在")
	}
	slots, _ := svc.Store.List()
	if len(slots) != 2 {
		t.Fatalf("应有 2 个槽位,得到 %d", len(slots))
	}
}

func TestDeleteAndRename(t *testing.T) {
	svc := setupTemp(t)
	writeFakeCredentials(t, "bigmodel", "alice")
	if _, err := svc.SaveCurrent("old", false); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.Rename("old", "new"); err != nil {
		t.Fatal(err)
	}
	if svc.Store.Has("old") || !svc.Store.Has("new") {
		t.Fatal("重命名结果不符")
	}
	if err := svc.Store.Delete("new"); err != nil {
		t.Fatal(err)
	}
	if svc.Store.Has("new") {
		t.Fatal("删除失败")
	}
}

func TestApplyMissingSlot(t *testing.T) {
	svc := setupTemp(t)
	if _, err := svc.Apply("nonexistent"); err == nil {
		t.Fatal("切换到不存在的槽位应报错")
	}
}

func TestCredentialsFilePerm(t *testing.T) {
	svc := setupTemp(t)
	writeFakeCredentials(t, "bigmodel", "alice")
	if _, err := svc.SaveCurrent("work", false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Apply("work"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(zcode.CredentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials.json 权限应为 0600,得到 %v", info.Mode().Perm())
	}
}
