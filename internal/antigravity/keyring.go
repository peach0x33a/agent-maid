// Linux Secret Service 的兼容层。
//
// 新版 agy CLI / Desktop 的主力凭据是 go-keyring 的 gemini/antigravity
// 条目(见 antigravity.go)。但 Linux 上部分 agy 版本把同一份凭据放在
// service=jetski-standalone-oauth-token 的 Secret Service 条目里;这里用
// secret-tool 枚举该 service,读取时作为主条目的兜底,切换时若条目已存在
// 则同步更新(保留原 label/username 属性),登出时删除。
//
// 钥匙串不可用(无 D-Bus/未装 secret-tool/被锁)时自动回落到旧版文件,
// 并做短时退避,避免每次刷新都等超时。
package antigravity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// altKeyringService agy 在 Secret Service 里使用的 service 属性。
	altKeyringService = "jetski-standalone-oauth-token"
	// keyringDefaultLabel 新建条目的默认标签(仅更新既有条目时参考)。
	keyringDefaultLabel = "Antigravity CLI"
	// keyringTimeout 单次 secret-tool 调用的超时。钥匙串被锁住时它会等
	// 用户解锁,不能让 TUI 卡死。
	keyringTimeout = 3 * time.Second
)

// keyringRunner 执行 secret-tool;测试里替换成假实现。
// stdin 是要保存的密钥(读取时为空),返回 stdout。
var keyringRunner = func(stdin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), keyringTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, secretToolPath(), args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("secret-tool %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

// secretToolPath 允许用 ANTIGRAVITY_SECRET_TOOL 覆盖 secret-tool 路径。
func secretToolPath() string {
	if p := strings.TrimSpace(os.Getenv("ANTIGRAVITY_SECRET_TOOL")); p != "" {
		return p
	}
	return "secret-tool"
}

// keyringEnabled 报告是否应尝试系统钥匙串:只在 Linux,且可用
// ANTIGRAVITY_KEYRING=0 关闭。
func keyringEnabled() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ANTIGRAVITY_KEYRING"))) {
	case "0", "off", "false", "no":
		return false
	}
	return true
}

// keyringItem 是 secret-tool 搜索到的一条钥匙串条目。
type keyringItem struct {
	attrs  map[string]string
	label  string
	secret string
}

// keyringItems 列出 service=altKeyringService 的全部条目。
// 钥匙串不可用(没有 D-Bus、没装 secret-tool、被锁定)时返回错误,
// 调用方据此回落到文件。失败后短时间内不再重试,避免每次刷新都等
// secret-tool 超时。
func keyringItems() ([]keyringItem, error) {
	if !keyringEnabled() {
		return nil, errors.New("系统钥匙串不可用")
	}
	if keyringCoolingDown() {
		return nil, errors.New("系统钥匙串暂不可用")
	}
	out, err := keyringRunner("", "search", "--all", "service", altKeyringService)
	if err != nil {
		// "没有条目"说明钥匙串本身可用,只是 agy 还没用它。
		if strings.Contains(strings.ToLower(err.Error()), "no such secret item") {
			markKeyringUsable()
			return nil, nil
		}
		markKeyringUnavailable()
		return nil, err
	}
	markKeyringUsable()
	return parseSecretToolSearch(out), nil
}

// 钥匙串不可用时的短时退避(进程内有效)。
var keyringHealth struct {
	mu           sync.Mutex
	unusableTill time.Time
}

func keyringCoolingDown() bool {
	keyringHealth.mu.Lock()
	defer keyringHealth.mu.Unlock()
	return time.Now().Before(keyringHealth.unusableTill)
}

func markKeyringUnavailable() {
	keyringHealth.mu.Lock()
	keyringHealth.unusableTill = time.Now().Add(5 * time.Minute)
	keyringHealth.mu.Unlock()
}

func markKeyringUsable() {
	keyringHealth.mu.Lock()
	keyringHealth.unusableTill = time.Time{}
	keyringHealth.mu.Unlock()
}

// parseSecretToolSearch 解析 secret-tool search --all 的输出:
//
//	[/org/freedesktop/secrets/collection/login/1]
//	label = Antigravity CLI
//	secret = {"token":...}
//	service = jetski-standalone-oauth-token
//	username = root
func parseSecretToolSearch(out string) []keyringItem {
	var items []keyringItem
	var cur *keyringItem
	flush := func() {
		if cur != nil && strings.TrimSpace(cur.secret) != "" {
			items = append(items, *cur)
		}
		cur = nil
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flush()
			cur = &keyringItem{attrs: map[string]string{}}
			continue
		}
		if cur == nil {
			continue
		}
		key, value, ok := strings.Cut(line, " = ")
		if !ok {
			continue
		}
		switch key {
		case "secret":
			cur.secret = value
		case "label":
			cur.label = value
		default:
			cur.attrs[key] = value
		}
	}
	flush()
	return items
}

// readKeyringToken 读取钥匙串里的 agy 凭据。没有条目或钥匙串不可用时
// 返回 ok=false。
func readKeyringToken() (Token, bool) {
	items, err := keyringItems()
	if err != nil {
		return nil, false
	}
	for _, item := range items {
		secret := strings.TrimSpace(item.secret)
		if json.Valid([]byte(secret)) {
			return Token(secret), true
		}
	}
	return nil, false
}

// writeKeyringToken 更新钥匙串里已有的 agy 凭据。没有任何条目时什么也
// 不做(返回 nil):此时 agy 本来就在用文件,不应凭空创建钥匙串条目。
func writeKeyringToken(tok Token) error {
	items, err := keyringItems()
	if err != nil || len(items) == 0 {
		return nil
	}
	// 更新 agy 实际会读到的条目:search 顺序里的第一条合法凭据。
	idx := 0
	for i, it := range items {
		if json.Valid([]byte(strings.TrimSpace(it.secret))) {
			idx = i
			break
		}
	}
	item := items[idx]
	label := item.label
	if label == "" {
		label = keyringDefaultLabel
	}
	attrs := item.attrs
	if _, ok := attrs["service"]; !ok {
		attrs["service"] = altKeyringService
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := []string{"store", "--label=" + label}
	for _, k := range keys {
		args = append(args, k, attrs[k])
	}
	if _, err := keyringRunner(string(tok), args...); err != nil {
		return fmt.Errorf("更新系统钥匙串失败: %w", err)
	}
	return nil
}

// deleteKeyringToken 删除钥匙串里的 agy 凭据(登出用)。钥匙串不可用时
// 当作没有条目处理。
func deleteKeyringToken() error {
	items, err := keyringItems()
	if err != nil {
		return nil
	}
	for _, item := range items {
		attrs := item.attrs
		if _, ok := attrs["service"]; !ok {
			attrs["service"] = altKeyringService
		}
		keys := make([]string, 0, len(attrs))
		for k := range attrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		args := []string{"clear"}
		for _, k := range keys {
			args = append(args, k, attrs[k])
		}
		if _, err := keyringRunner("", args...); err != nil {
			return fmt.Errorf("清除系统钥匙串失败: %w", err)
		}
	}
	return nil
}
