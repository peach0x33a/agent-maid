package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setup 把所有数据目录指到临时目录,并返回一个可复用的 Env。
func setup(t *testing.T) (Env, *bytes.Buffer) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("AGENT_MAID_DIR", filepath.Join(base, "cfg"))
	t.Setenv("ZCODE_V2_DIR", filepath.Join(base, "zcode"))
	t.Setenv("ZCODE_ACC_SWITCH_DIR", filepath.Join(base, "zslots"))
	t.Setenv("CODEX_HOME", filepath.Join(base, "codex"))
	t.Setenv("ANTIGRAVITY_GEMINI_DIR", filepath.Join(base, "gemini"))
	t.Setenv("ANTIGRAVITY_CLI_DIR", "")
	t.Setenv("ANTIGRAVITY_STORE_DIR", filepath.Join(base, "aslots"))
	out := &bytes.Buffer{}
	return Env{Stdout: out, Stderr: out, Stdin: strings.NewReader("")}, out
}

// run 执行命令并返回退出码与本次输出(每次调用清空缓冲)。
func run(env Env, args ...string) (int, string) {
	buf := env.Stdout.(*bytes.Buffer)
	buf.Reset()
	code := Run(args, env)
	return code, buf.String()
}

func jwt(payload map[string]any) string {
	enc := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return enc(map[string]any{"alg": "RS256"}) + "." + enc(payload) + ".sig"
}

// writeCodexAuth 在 CODEX_HOME 里放一份可解析的 auth.json。
func writeCodexAuth(t *testing.T, accountID, email string) {
	t.Helper()
	home := os.Getenv("CODEX_HOME")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	at := jwt(map[string]any{
		"exp":                         1790000000,
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": accountID, "chatgpt_plan_type": "plus"},
	})
	doc := map[string]any{
		"auth_mode":      "chatgpt",
		"OPENAI_API_KEY": nil,
		"tokens": map[string]any{
			"id_token":      jwt(map[string]any{"email": email}),
			"access_token":  at,
			"refresh_token": "rt-" + accountID,
			"account_id":    accountID,
		},
		"last_refresh": "2026-10-26T00:00:00Z",
	}
	raw, _ := json.Marshal(doc)
	if err := os.WriteFile(filepath.Join(home, "auth.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func decode[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &v); err != nil {
		t.Fatalf("输出不是合法 JSON: %v\n%s", err, s)
	}
	return v
}

type errPayload struct {
	Type  string `json:"type"`
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// 用法错误统一 exit 2,便于脚本区分"用错"与"执行失败"。
func TestUsageErrorsExitTwo(t *testing.T) {
	env, _ := setup(t)
	cases := [][]string{
		{"list"},
		{"list", "nosuch"},
		{"list", "chatgpt", "extra"},
		{"use"},
		{"use", "chatgpt"},
		{"rm", "zcode"},
		{"import", "chatgpt"},
		{"import", "zcode", "x.json"},
		{"save", "nosuch"},
	}
	for _, args := range cases {
		code, out := run(env, args...)
		if code != CodeUsage {
			t.Errorf("%v: 退出码 %d, 期望 %d", args, code, CodeUsage)
		}
		if strings.TrimSpace(out) == "" {
			t.Errorf("%v: 没有输出任何说明", args)
		}
	}
}

// 执行失败(找不到目标等)exit 1,并给出原因。
func TestFailureExitsOne(t *testing.T) {
	env, _ := setup(t)
	code, out := run(env, "use", "chatgpt", "nope")
	if code != CodeFailure {
		t.Fatalf("退出码 %d, 期望 %d", code, CodeFailure)
	}
	if !strings.Contains(out, "找不到") {
		t.Fatalf("输出没有说明原因: %s", out)
	}
}

// list 在没有任何数据时也要 exit 0(空列表不是错误)。
func TestListEmptyIsOK(t *testing.T) {
	env, _ := setup(t)
	for _, target := range []string{"chatgpt", "zcode", "antigravity"} {
		code, out := run(env, "list", target)
		if code != CodeOK {
			t.Errorf("%s: 退出码 %d, 期望 0", target, code)
		}
		if strings.TrimSpace(out) == "" {
			t.Errorf("%s: 空列表也要给出引导文案", target)
		}
	}
}

// --json 出现在任意位置都要生效,且字段名稳定。
func TestJSONFlagAnywhere(t *testing.T) {
	env, _ := setup(t)
	writeCodexAuth(t, "acc-1", "a@example.com")

	for _, args := range [][]string{
		{"list", "chatgpt", "--json"},
		{"--json", "list", "chatgpt"},
		{"list", "--json", "chatgpt"},
	} {
		code, out := run(env, args...)
		if code != CodeOK {
			t.Fatalf("%v: 退出码 %d\n%s", args, code, out)
		}
		v := decode[struct {
			Type  string `json:"type"`
			OK    bool   `json:"ok"`
			Total int    `json:"total"`
		}](t, out)
		if v.Type != "list" || !v.OK {
			t.Fatalf("%v: JSON 外壳不对: %s", args, out)
		}
	}
}

// 账号的完整生命周期:save → list → use → rm。
func TestChatgptLifecycle(t *testing.T) {
	env, _ := setup(t)
	writeCodexAuth(t, "acc-1", "a@example.com")

	if code, out := run(env, "save", "chatgpt", "work"); code != CodeOK {
		t.Fatalf("save: rc=%d %s", code, out)
	}
	code, out := run(env, "list", "chatgpt", "--json")
	if code != CodeOK {
		t.Fatalf("list: rc=%d %s", code, out)
	}
	listed := decode[struct {
		Total    int `json:"total"`
		Accounts []struct {
			Label string `json:"label"`
			InUse bool   `json:"in_use"`
		} `json:"accounts"`
		Current struct {
			InLibrary bool `json:"in_library"`
		} `json:"current"`
	}](t, out)
	if listed.Total != 1 || listed.Accounts[0].Label != "work" {
		t.Fatalf("账号库内容不对: %s", out)
	}
	if !listed.Accounts[0].InUse {
		t.Fatal("刚保存的账号应标为使用中")
	}
	if !listed.Current.InLibrary {
		t.Fatal("已保存的当前账号应报告 in_library=true")
	}

	// 登出后再 use 应该把凭据写回来
	if code, out := run(env, "logout", "chatgpt"); code != CodeOK {
		t.Fatalf("logout: rc=%d %s", code, out)
	}
	if code, out := run(env, "use", "chatgpt", "work"); code != CodeOK {
		t.Fatalf("use: rc=%d %s", code, out)
	}
	authPath := filepath.Join(os.Getenv("CODEX_HOME"), "auth.json")
	raw, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("use 之后 auth.json 应存在: %v", err)
	}
	if !strings.Contains(string(raw), "acc-1") {
		t.Fatalf("auth.json 内容不对: %s", raw)
	}

	if code, out := run(env, "rm", "chatgpt", "work"); code != CodeOK {
		t.Fatalf("rm: rc=%d %s", code, out)
	}
	if code, _ := run(env, "list", "chatgpt", "--json"); code != CodeOK {
		t.Fatal("rm 之后 list 仍应成功")
	}
}

// 保存时不给名字应保留用户之前改过的显示名。
func TestSaveKeepsCustomLabel(t *testing.T) {
	env, _ := setup(t)
	writeCodexAuth(t, "acc-1", "a@example.com")
	if code, out := run(env, "save", "chatgpt", "我的名字"); code != CodeOK {
		t.Fatalf("save: rc=%d %s", code, out)
	}
	if code, out := run(env, "save", "chatgpt"); code != CodeOK {
		t.Fatalf("save(默认名): rc=%d %s", code, out)
	}
	code, out := run(env, "list", "chatgpt", "--json")
	if code != CodeOK {
		t.Fatal(out)
	}
	v := decode[struct {
		Accounts []struct {
			Label string `json:"label"`
		} `json:"accounts"`
	}](t, out)
	if v.Accounts[0].Label != "我的名字" {
		t.Fatalf("显示名被覆盖为 %q", v.Accounts[0].Label)
	}
}

// 从 stdin 导入多账号(JSONL),并保证不自动激活。
func TestImportFromStdin(t *testing.T) {
	env, _ := setup(t)
	lines := []string{}
	for _, id := range []string{"acc-a", "acc-b"} {
		at := jwt(map[string]any{
			"exp":                         1790000000,
			"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": id},
		})
		line, _ := json.Marshal(map[string]any{
			"accessToken":  at,
			"refreshToken": "rt-" + id,
			"user":         map[string]any{"email": id + "@example.com"},
		})
		lines = append(lines, string(line))
	}
	env.Stdin = strings.NewReader(strings.Join(lines, "\n"))

	code, out := run(env, "import", "chatgpt", "-")
	if code != CodeOK {
		t.Fatalf("import: rc=%d %s", code, out)
	}
	code, out = run(env, "list", "chatgpt", "--json")
	if code != CodeOK {
		t.Fatal(out)
	}
	v := decode[struct {
		Total    int `json:"total"`
		Accounts []struct {
			InUse bool `json:"in_use"`
		} `json:"accounts"`
	}](t, out)
	if v.Total != 2 {
		t.Fatalf("应导入 2 个账号,实际 %d: %s", v.Total, out)
	}
	for _, a := range v.Accounts {
		if a.InUse {
			t.Fatal("导入不应自动激活任何账号")
		}
	}
}

// 导入无效内容要 exit 1 并说明,而不是静默成功。
func TestImportInvalidInput(t *testing.T) {
	env, _ := setup(t)
	env.Stdin = strings.NewReader(`{"hello":"world"}`)
	code, out := run(env, "import", "chatgpt", "-")
	if code != CodeFailure {
		t.Fatalf("退出码 %d, 期望 %d", code, CodeFailure)
	}
	if !strings.Contains(out, "access_token") {
		t.Fatalf("应说明没找到账号对象: %s", out)
	}
}

// 非 chatgpt 目标不支持 import,要给出替代做法。
func TestImportRejectsSlotTargets(t *testing.T) {
	env, _ := setup(t)
	for _, target := range []string{"zcode", "antigravity"} {
		code, out := run(env, "import", target, "x.json")
		if code != CodeUsage {
			t.Fatalf("%s: 退出码 %d", target, code)
		}
		if !strings.Contains(out, "save") {
			t.Fatalf("%s: 提示里应说明改用 save: %s", target, out)
		}
	}
}

// ZCode 槽位:save → list → use。
func TestZcodeSlots(t *testing.T) {
	env, _ := setup(t)
	dir := os.Getenv("ZCODE_V2_DIR")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	creds := map[string]string{
		"oauth:active_provider":  "github",
		"oauth:github:user_info": `{"id":"u1","username":"alice","displayName":"Alice"}`,
	}
	raw, _ := json.Marshal(creds)
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if code, out := run(env, "save", "zcode", "work"); code != CodeOK {
		t.Fatalf("save: rc=%d %s", code, out)
	}
	code, out := run(env, "list", "zcode", "--json")
	if code != CodeOK {
		t.Fatal(out)
	}
	v := decode[struct {
		Total int `json:"total"`
		Slots []struct {
			Name  string `json:"name"`
			InUse bool   `json:"in_use"`
		} `json:"slots"`
	}](t, out)
	if v.Total != 1 || v.Slots[0].Name != "work" || !v.Slots[0].InUse {
		t.Fatalf("槽位列表不对: %s", out)
	}
	if code, out := run(env, "use", "zcode", "work"); code != CodeOK {
		t.Fatalf("use: rc=%d %s", code, out)
	}
}

// Antigravity:save → list → use,并报告凭据时效。
func TestAntigravitySlots(t *testing.T) {
	env, _ := setup(t)
	cliDir := filepath.Join(os.Getenv("ANTIGRAVITY_GEMINI_DIR"), "antigravity-cli")
	if err := os.MkdirAll(cliDir, 0o700); err != nil {
		t.Fatal(err)
	}
	doc, _ := json.Marshal(map[string]any{
		"token": map[string]any{
			"access_token":  "at-1",
			"token_type":    "Bearer",
			"refresh_token": "rt-1",
			"expiry":        "2030-01-01T00:00:00Z",
		},
		"auth_method":    "consumer",
		"id_token_email": "agy@example.com",
	})
	if err := os.WriteFile(filepath.Join(cliDir, "antigravity-oauth-token"), doc, 0o600); err != nil {
		t.Fatal(err)
	}

	if code, out := run(env, "save", "antigravity", "work"); code != CodeOK {
		t.Fatalf("save: rc=%d %s", code, out)
	}
	code, out := run(env, "list", "antigravity", "--json")
	if code != CodeOK {
		t.Fatal(out)
	}
	v := decode[struct {
		Total int `json:"total"`
		Slots []struct {
			Name        string `json:"name"`
			Email       string `json:"email"`
			Refreshable bool   `json:"refreshable"`
			InUse       bool   `json:"in_use"`
		} `json:"slots"`
	}](t, out)
	if v.Total != 1 || v.Slots[0].Email != "agy@example.com" || !v.Slots[0].Refreshable {
		t.Fatalf("槽位信息不对: %s", out)
	}
	if code, out := run(env, "use", "antigravity", "work"); code != CodeOK {
		t.Fatalf("use: rc=%d %s", code, out)
	}
}

// 目标名大小写与别名都要可用。
func TestTargetAliases(t *testing.T) {
	env, _ := setup(t)
	for _, target := range []string{"chatgpt", "ChatGPT", "gpt", "codex"} {
		if code, out := run(env, "list", target); code != CodeOK {
			t.Errorf("%s: rc=%d %s", target, code, out)
		}
	}
	for _, target := range []string{"zcode", "ZCODE", "z"} {
		if code, out := run(env, "list", target); code != CodeOK {
			t.Errorf("%s: rc=%d %s", target, code, out)
		}
	}
	for _, target := range []string{"antigravity", "antigravity", "ANTIGRAVITY", "agy", "ag", "anti"} {
		if code, out := run(env, "list", target); code != CodeOK {
			t.Errorf("%s: rc=%d %s", target, code, out)
		}
	}
}

// IsCommand 必须只认命令名,不能把 TUI 的启动参数当命令。
func TestIsCommand(t *testing.T) {
	commands := []string{"list", "ls", "current", "use", "save", "rm", "import", "logout"}
	for _, c := range commands {
		if !IsCommand([]string{c}) {
			t.Errorf("%q 应被识别为命令", c)
		}
	}
	for _, a := range []string{"chatgpt", "zcode", "antigravity", "version", "--help", "help", ""} {
		if IsCommand([]string{a}) {
			t.Errorf("%q 不应被识别为命令", a)
		}
	}
}

// save/use 的 JSON 必须自洽:refreshable 反映真实凭据,in_use 表示"现在生效"。
// (回归测试:这两处曾分别误报 false 和漏填)
func TestSaveUseJSONConsistency(t *testing.T) {
	env, _ := setup(t)
	writeCodexAuth(t, "acc-1", "a@example.com")

	code, out := run(env, "save", "chatgpt", "work", "--json")
	if code != CodeOK {
		t.Fatalf("save: rc=%d %s", code, out)
	}
	saved := decode[struct {
		Account struct {
			Refreshable bool `json:"refreshable"`
			InUse       bool `json:"in_use"`
		} `json:"account"`
	}](t, out)
	if !saved.Account.Refreshable {
		t.Fatal("save: 该凭据有 refresh_token,refreshable 应为 true")
	}
	if !saved.Account.InUse {
		t.Fatal("save: 刚保存的就是当前生效账号,in_use 应为 true")
	}

	code, out = run(env, "use", "chatgpt", "work", "--json")
	if code != CodeOK {
		t.Fatalf("use: rc=%d %s", code, out)
	}
	used := decode[struct {
		Account struct {
			Refreshable bool `json:"refreshable"`
			InUse       bool `json:"in_use"`
		} `json:"account"`
	}](t, out)
	if !used.Account.Refreshable {
		t.Fatal("use: refreshable 应为 true")
	}
	if !used.Account.InUse {
		t.Fatal("use: 激活后 in_use 应为 true")
	}
}

// use 默认必须拒绝切换"当前账号未保存"的情形,并给出可执行的下一步。
// 否则原账号只剩一份没人认得的凭据/auto-* 槽位,再也切不回来。
func TestUseRefusesUnsavedCurrent(t *testing.T) {
	env, _ := setup(t)
	writeCodexAuth(t, "acc-1", "a@example.com")

	// 先存两个账号:一个是当前登录的 acc-1,另一个是待切换的 acc-2
	if code, out := run(env, "save", "chatgpt", "current-one"); code != CodeOK {
		t.Fatalf("save current: rc=%d %s", code, out)
	}
	writeCodexAuth(t, "acc-2", "b@example.com")
	if code, out := run(env, "save", "chatgpt", "other"); code != CodeOK {
		t.Fatalf("save other: rc=%d %s", code, out)
	}

	// 现在官方端登录的是 acc-2(未在库里?它已入库) —— 构造"未保存"状态:
	// 直接换一个全新的官方凭据,不入库
	writeCodexAuth(t, "acc-3", "c@example.com")

	code, out := run(env, "use", "chatgpt", "current-one")
	if code != CodeFailure {
		t.Fatalf("当前账号未保存时应拒绝切换,退出码 %d\n%s", code, out)
	}
	if !strings.Contains(out, "尚未保存") {
		t.Fatalf("应说明原因: %s", out)
	}
	if !strings.Contains(out, "save") {
		t.Fatalf("应给出 save 的下一步: %s", out)
	}

	// 官方 auth.json 不能被改动
	raw, err := os.ReadFile(filepath.Join(os.Getenv("CODEX_HOME"), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "acc-3") {
		t.Fatal("被拒绝的 use 不应修改 auth.json")
	}
}

// --force 才允许越过守卫,并且仍然自动备份。
func TestUseForceAllowsUnsaved(t *testing.T) {
	env, _ := setup(t)
	writeCodexAuth(t, "acc-1", "a@example.com")
	if code, out := run(env, "save", "chatgpt", "saved-one"); code != CodeOK {
		t.Fatalf("save: rc=%d %s", code, out)
	}
	writeCodexAuth(t, "acc-9", "new@example.com") // 未保存

	code, out := run(env, "use", "chatgpt", "saved-one", "--force")
	if code != CodeOK {
		t.Fatalf("--force 应允许切换: rc=%d %s", code, out)
	}
	raw, _ := os.ReadFile(filepath.Join(os.Getenv("CODEX_HOME"), "auth.json"))
	if !strings.Contains(string(raw), "acc-1") {
		t.Fatalf("切换后 auth.json 应为 acc-1: %s", raw)
	}
}

// 拒绝时 --json 要带结构化的补救信息,agent 才好自动继续。
func TestUseRefusesUnsavedJSON(t *testing.T) {
	env, _ := setup(t)
	writeCodexAuth(t, "acc-1", "a@example.com")
	if code, _ := run(env, "save", "chatgpt", "one"); code != CodeOK {
		t.Fatal("save 失败")
	}
	writeCodexAuth(t, "acc-2", "b@example.com")

	code, out := run(env, "use", "chatgpt", "one", "--json")
	if code != CodeFailure {
		t.Fatalf("退出码 %d, 期望 %d", code, CodeFailure)
	}
	v := decode[struct {
		OK        bool   `json:"ok"`
		Error     string `json:"error"`
		Reason    string `json:"reason"`
		Needs     string `json:"needs"`
		ForceFlag string `json:"force_flag"`
	}](t, out)
	if v.OK || v.Reason != "unsaved_current" || v.Needs != "save" || v.ForceFlag != "--force" {
		t.Fatalf("结构化提示不对: %s", out)
	}
	if v.Error == "" {
		t.Fatal("error 不能为空")
	}
}

// ZCode / agy 的槽位切换同样默认拒绝未保存的当前账号。
func TestSlotUseRefusesUnsavedCurrent(t *testing.T) {
	env, _ := setup(t)

	// ZCode
	zdir := os.Getenv("ZCODE_V2_DIR")
	if err := os.MkdirAll(zdir, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(user string) {
		creds := map[string]string{
			"oauth:active_provider":  "github",
			"oauth:github:user_info": `{"id":"u1","username":"` + user + `"}`,
		}
		raw, _ := json.Marshal(creds)
		if err := os.WriteFile(filepath.Join(zdir, "credentials.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("alice")
	if code, out := run(env, "save", "zcode", "alice"); code != CodeOK {
		t.Fatalf("zcode save: rc=%d %s", code, out)
	}
	write("bob") // 当前是 bob,未保存
	if code, out := run(env, "use", "zcode", "alice"); code != CodeFailure {
		t.Fatalf("zcode 未保存时应拒绝: rc=%d %s", code, out)
	}
	if code, out := run(env, "use", "zcode", "alice", "--force"); code != CodeOK {
		t.Fatalf("zcode --force 应允许: rc=%d %s", code, out)
	}

	// Antigravity
	cdir := filepath.Join(os.Getenv("ANTIGRAVITY_GEMINI_DIR"), "antigravity-cli")
	if err := os.MkdirAll(cdir, 0o700); err != nil {
		t.Fatal(err)
	}
	tokPath := filepath.Join(cdir, "antigravity-oauth-token")
	writeTok := func(email string) {
		doc, _ := json.Marshal(map[string]any{
			"token":          map[string]any{"access_token": "at-" + email, "refresh_token": "rt"},
			"auth_method":    "consumer",
			"id_token_email": email,
		})
		if err := os.WriteFile(tokPath, doc, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeTok("a@example.com")
	if code, out := run(env, "save", "antigravity", "a"); code != CodeOK {
		t.Fatalf("agy save: rc=%d %s", code, out)
	}
	writeTok("b@example.com")
	if code, out := run(env, "use", "antigravity", "a"); code != CodeFailure {
		t.Fatalf("agy 未保存时应拒绝: rc=%d %s", code, out)
	}
	if code, out := run(env, "use", "antigravity", "a", "--force"); code != CodeOK {
		t.Fatalf("agy --force 应允许: rc=%d %s", code, out)
	}
}

// 拼错的选项必须报"未知选项",不能当成账号名去查库。
func TestUnknownFlagRejected(t *testing.T) {
	env, _ := setup(t)
	code, out := run(env, "use", "chatgpt", "x", "--frce")
	if code != CodeUsage {
		t.Fatalf("退出码 %d, 期望 %d", code, CodeUsage)
	}
	if !strings.Contains(out, "未知选项") {
		t.Fatalf("应指出是未知选项: %s", out)
	}
}
