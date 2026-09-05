// Package meta holds the program identity and the unified config layout, so
// both target managers (ChatGPT / ZCode) share one directory and the binary
// name stays in sync with its on-disk paths.
package meta

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	Name    = "agent-maid"
	Version = "0.1.0"
)

// Dir is the base config directory: $AGENT_MAID_DIR, then $XDG_CONFIG_HOME,
// then ~/.config/agent-maid.
func Dir() string {
	if d := strings.TrimSpace(os.Getenv("AGENT_MAID_DIR")); d != "" {
		return d
	}
	if base := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); base != "" {
		return filepath.Join(base, Name)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", Name)
}

// GPTStorePath is the ChatGPT account library file.
func GPTStorePath() string { return filepath.Join(Dir(), "chatgpt", "store.json") }

// ZcodeBaseDir is the root of the ZCode slot store; slots live one level down
// in slots/ (zcodestore appends that itself).
func ZcodeBaseDir() string { return filepath.Join(Dir(), "zcode") }

// MigrateLegacy copies data from the two predecessor tools on first run, so
// existing account libraries and slots carry over untouched. Best effort:
// any failure is silently ignored and the new layout starts empty.
//
//	~/.config/gptswitch/store.json            → chatgpt/store.json
//	~/.config/chatgpt-app-account-switch/...  → (same, second candidate)
//	~/.config/zcode-acc-switch/slots/*.json   → zcode/slots/*.json
func MigrateLegacy() {
	if d := os.Getenv("ZCODE_ACC_SWITCH_DIR"); d != "" {
		// 用户显式指定了槽位目录:完全沿用,不做迁移
	} else {
		migrateDir(
			legacyConfigDir("zcode-acc-switch"),
			filepath.Join(ZcodeBaseDir(), "slots"),
		)
	}
	for _, old := range []string{
		filepath.Join(legacyConfigDir("gptswitch"), "store.json"),
		filepath.Join(legacyConfigDir("chatgpt-app-account-switch"), "store.json"),
	} {
		if migrateFile(old, GPTStorePath()) {
			break
		}
	}
}

// legacyConfigDir mirrors the old tools' ~/.config/<name> resolution
// (XDG_CONFIG_HOME respected).
func legacyConfigDir(name string) string {
	if base := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); base != "" {
		return filepath.Join(base, name)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", name)
}

func migrateFile(src, dst string) bool {
	if _, err := os.Stat(dst); err == nil {
		return false // 目标已存在,绝不覆盖
	} else if !os.IsNotExist(err) {
		return false
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return false
	}
	return os.WriteFile(dst, data, 0o600) == nil
}

func migrateDir(srcSlots, dstSlots string) {
	entries, err := os.ReadDir(filepath.Join(srcSlots, "slots"))
	if err != nil {
		return
	}
	if err := os.MkdirAll(dstSlots, 0o700); err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		src := filepath.Join(srcSlots, "slots", e.Name())
		dst := filepath.Join(dstSlots, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		copyFile(src, dst)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fi.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
