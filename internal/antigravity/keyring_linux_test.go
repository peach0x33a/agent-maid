//go:build linux

package antigravity

import (
	"errors"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// fakeKeyring 模拟 Secret Service:search/store/clear 三种调用。
type fakeKeyring struct {
	items       []keyringItem
	storeCalls  int
	searchCalls int
	failSearch  bool
}

func (f *fakeKeyring) run(stdin string, args ...string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("missing command")
	}
	switch args[0] {
	case "search":
		f.searchCalls++
		if f.failSearch {
			return "", errors.New("Cannot autolaunch D-Bus without X11 $DISPLAY")
		}
		if len(f.items) == 0 {
			return "", errors.New("No such secret item at path")
		}
		var b strings.Builder
		for _, item := range f.items {
			b.WriteString("[/org/freedesktop/secrets/collection/login/1]\n")
			if item.label != "" {
				b.WriteString("label = " + item.label + "\n")
			}
			b.WriteString("secret = " + item.secret + "\n")
			keys := make([]string, 0, len(item.attrs))
			for k := range item.attrs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				b.WriteString(k + " = " + item.attrs[k] + "\n")
			}
		}
		return b.String(), nil
	case "store":
		f.storeCalls++
		attrs := map[string]string{}
		label := ""
		rest := args[1:]
		for i := 0; i < len(rest); i++ {
			if strings.HasPrefix(rest[i], "--label=") {
				label = strings.TrimPrefix(rest[i], "--label=")
				continue
			}
			if i+1 < len(rest) {
				attrs[rest[i]] = rest[i+1]
				i++
			}
		}
		f.items = []keyringItem{{attrs: attrs, label: label, secret: stdin}}
		return "", nil
	case "clear":
		f.items = nil
		return "", nil
	}
	return "", errors.New("unexpected command " + args[0])
}

func useFakeKeyring(t *testing.T, f *fakeKeyring) {
	t.Helper()
	keyring.MockInit() // 主钥匙串(gemini/antigravity)用空 mock,避免碰真实钥匙串
	prev := keyringRunner
	keyringRunner = f.run
	markKeyringUsable()
	t.Cleanup(func() {
		keyringRunner = prev
		markKeyringUsable()
	})
	t.Setenv("ANTIGRAVITY_KEYRING", "1")
}

func keyringItemFor(tok string) keyringItem {
	return keyringItem{
		attrs: map[string]string{
			"service":  keyringService,
			"username": "root",
		},
		label:  keyringDefaultLabel,
		secret: tok,
	}
}

func TestParseSecretToolSearch(t *testing.T) {
	out := `[/org/freedesktop/secrets/collection/login/1]
label = Antigravity CLI
secret = {"token":{"access_token":"a"}}
service = jetski-standalone-oauth-token
username = root

[/org/freedesktop/secrets/collection/login/2]
label = other
secret = not-json
service = jetski-standalone-oauth-token
username = root
`
	items := parseSecretToolSearch(out)
	if len(items) != 2 {
		t.Fatalf("应解析出 2 条条目,得到 %d", len(items))
	}
	if items[0].attrs["username"] != "root" || items[0].label != "Antigravity CLI" {
		t.Fatalf("条目属性不对: %+v", items[0])
	}
	if !strings.Contains(items[0].secret, "access_token") {
		t.Fatalf("secret 解析不对: %q", items[0].secret)
	}
}

// 钥匙串不可用后短时间退避:连续刷新不应反复等 secret-tool 超时。
func TestKeyringCooldownAfterFailure(t *testing.T) {
	setupIsolated(t)
	f := &fakeKeyring{failSearch: true}
	useFakeKeyring(t, f)

	if _, err := keyringItems(); err == nil {
		t.Fatal("第一次探测应失败")
	}
	// 让后续调用本来会成功;退避期内不应再次调用 secret-tool。
	f.failSearch = false
	f.items = []keyringItem{keyringItemFor(`{"token":{"access_token":"a"}}`)}
	if _, err := keyringItems(); err == nil {
		t.Fatal("退避期内应直接回落,不应重试成功")
	}
	if f.searchCalls != 1 {
		t.Fatalf("退避期内不应重复调用 secret-tool,实际 %d 次", f.searchCalls)
	}
	markKeyringUsable()
	if items, err := keyringItems(); err != nil || len(items) != 1 {
		t.Fatalf("解除退避后应能读到条目: %v %+v", err, items)
	}
}

// agy 优先读钥匙串:钥匙串有条目时,即使文件里是另一个账号也用钥匙串。
func TestKeyringPreferredOverFile(t *testing.T) {
	setupIsolated(t)
	f := &fakeKeyring{}
	useFakeKeyring(t, f)
	f.items = []keyringItem{keyringItemFor(`{"token":{"access_token":"from-keyring"},"id_token_email":"ring@example.com"}`)}
	writeTokenFile(t, tokenDocFor("from-file", "rt", "file@example.com", time.Now().Add(time.Hour)))

	tok, source, err := ReadTokenWithSource()
	if err != nil {
		t.Fatal(err)
	}
	if source != SourceKeyring {
		t.Fatalf("来源 = %q, 期望钥匙串", source)
	}
	if TokenEmail(tok) != "ring@example.com" {
		t.Fatalf("应读取钥匙串凭据,得到 %s", tok)
	}
}

// 钥匙串不可用时安静回落到文件。
func TestKeyringFailureFallsBackToFile(t *testing.T) {
	setupIsolated(t)
	f := &fakeKeyring{failSearch: true}
	useFakeKeyring(t, f)
	writeTokenFile(t, tokenDocFor("from-file", "rt", "file@example.com", time.Now().Add(time.Hour)))

	tok, source, err := ReadTokenWithSource()
	if err != nil {
		t.Fatal(err)
	}
	if source != SourceFile || !strings.Contains(string(tok), "from-file") {
		t.Fatalf("应回落到文件,得到 source=%q tok=%s", source, tok)
	}
}

// 钥匙串里已有 agy 条目时,切换账号要同步更新它,否则 agy 仍读旧账号。
func TestApplyUpdatesKeyring(t *testing.T) {
	setupIsolated(t)
	f := &fakeKeyring{}
	useFakeKeyring(t, f)
	exp := time.Now().Add(time.Hour).Round(time.Second)

	// 槽位里存 A
	writeTokenFile(t, tokenDocFor("access-A", "rt-A", "a@example.com", exp))
	svc := New()
	if _, err := svc.SaveCurrent("work", false); err != nil {
		t.Fatal(err)
	}

	// agy 当前在钥匙串里是 B(文件里也放 B)
	writeTokenFile(t, tokenDocFor("access-B", "rt-B", "b@example.com", exp))
	f.items = []keyringItem{keyringItemFor(`{"token":{"access_token":"access-B","refresh_token":"rt-B","expiry":"` + exp.Format(time.RFC3339Nano) + `"},"auth_method":"consumer","id_token_email":"b@example.com"}`)}

	backup, err := svc.Apply("work")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if backup == "" {
		t.Fatal("钥匙串里的未保存账号应先自动备份")
	}
	if f.storeCalls != 1 {
		t.Fatalf("应更新一次钥匙串,实际 %d", f.storeCalls)
	}
	if len(f.items) != 1 || !strings.Contains(f.items[0].secret, "access-A") {
		t.Fatalf("钥匙串未切换到 A: %+v", f.items)
	}
	if f.items[0].attrs["username"] != "root" || f.items[0].attrs["service"] != keyringService {
		t.Fatalf("更新时应保留原属性: %+v", f.items[0].attrs)
	}
}

// 钥匙串里没有另一套条目时不要凭空创建;主条目(gemini/antigravity)照常写入,
// 旧版凭据文件不存在时也不主动创建(与 macOS 共用凭据的行为一致)。
func TestWriteUsesPrimaryAndDoesNotCreateAlternate(t *testing.T) {
	setupIsolated(t)
	f := &fakeKeyring{}
	useFakeKeyring(t, f)
	tok := `{"token":{"access_token":"a"},"id_token_email":"a@example.com"}`
	if err := WriteToken(Token(tok)); err != nil {
		t.Fatal(err)
	}
	if f.storeCalls != 0 {
		t.Fatalf("没有既有条目时不应写另一套 Secret Service,实际 %d 次", f.storeCalls)
	}
	got, err := keyring.Get(keyringService, keyringAccount)
	if err != nil || !strings.Contains(got, "access_token") {
		t.Fatalf("主钥匙串未写入: %q %v", got, err)
	}
	if _, err := os.Stat(TokenPath()); !os.IsNotExist(err) {
		t.Fatal("Keyring 模式下不应凭空创建旧版凭据文件")
	}
}

// 主钥匙串(gemini/antigravity)优先于 Linux 的另一套 Secret Service 条目。
func TestPrimaryKeyringPreferredOverAlternate(t *testing.T) {
	setupIsolated(t)
	f := &fakeKeyring{}
	useFakeKeyring(t, f)
	f.items = []keyringItem{keyringItemFor(`{"token":{"access_token":"alt"},"id_token_email":"alt@example.com"}`)}
	if err := keyring.Set(keyringService, keyringAccount, `{"token":{"access_token":"primary"},"id_token_email":"primary@example.com"}`); err != nil {
		t.Fatal(err)
	}
	tok, source, err := ReadTokenWithSource()
	if err != nil {
		t.Fatal(err)
	}
	if source != SourceKeyring || TokenEmail(tok) != "primary@example.com" {
		t.Fatalf("应优先读主钥匙串,得到 source=%q tok=%s", source, tok)
	}
}

// 登出要同时清掉钥匙串条目,否则 agy 下次仍会用钥匙串自动登录。
func TestClearTokenRemovesKeyring(t *testing.T) {
	setupIsolated(t)
	f := &fakeKeyring{}
	useFakeKeyring(t, f)
	f.items = []keyringItem{keyringItemFor(`{"token":{"access_token":"a"}}`)}
	writeTokenFile(t, tokenDocFor("a", "r", "a@example.com", time.Now().Add(time.Hour)))

	if err := ClearToken(); err != nil {
		t.Fatal(err)
	}
	if len(f.items) != 0 {
		t.Fatalf("钥匙串条目应被清除: %+v", f.items)
	}
	if _, err := os.Stat(TokenPath()); !os.IsNotExist(err) {
		t.Fatal("凭据文件应已改名")
	}
	entries, err := os.ReadDir(CLIDir())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "."+tokenFileName+".logged-out-") {
			found = true
		}
	}
	if !found {
		t.Fatalf("登出应留下文件备份,目录: %v", entries)
	}
}

// 钥匙串不可用(无 D-Bus / 未激活)时,读写都应回落文件而不是报错。
func TestKeyringUnavailableFallsBackFile(t *testing.T) {
	setupIsolated(t)
	t.Setenv("ANTIGRAVITY_KEYRING", "1")
	prev := kr
	kr = brokenKeyring{}
	t.Cleanup(func() { kr = prev })

	tok := `{"token":{"access_token":"file-only"},"id_token_email":"file@example.com"}`
	if err := WriteToken(Token(tok)); err != nil {
		t.Fatalf("钥匙串不可用时 WriteToken 应回落文件: %v", err)
	}
	raw, err := os.ReadFile(TokenPath())
	if err != nil || !strings.Contains(string(raw), "file-only") {
		t.Fatalf("应写入凭据文件: %q %v", raw, err)
	}
	if err := ClearToken(); err != nil {
		t.Fatalf("钥匙串不可用时 ClearToken 应只处理文件: %v", err)
	}
	if _, err := os.Stat(TokenPath()); !os.IsNotExist(err) {
		t.Fatal("登出后文件应已改名")
	}
}

type brokenKeyring struct{}

func (brokenKeyring) Get(string, string) (string, error) {
	return "", errors.New("The name is not activatable")
}
func (brokenKeyring) Set(string, string, string) error {
	return errors.New("The name is not activatable")
}
func (brokenKeyring) Delete(string, string) error {
	return errors.New("The name is not activatable")
}

// 非 Linux 平台不探测钥匙串(仅编译期覆盖,linux 构建下直接跳过)。
func TestKeyringDisabled(t *testing.T) {
	t.Setenv("ANTIGRAVITY_KEYRING", "0")
	if keyringEnabled() {
		t.Fatal("ANTIGRAVITY_KEYRING=0 时应关闭钥匙串")
	}
	if _, err := keyringItems(); err == nil {
		t.Fatal("关闭时 keyringItems 应返回错误")
	}
}
