// Package meta holds the program identity and the unified config layout, so
// both target managers (ChatGPT / ZCode) share one directory and the binary
// name stays in sync with its on-disk paths.
package meta

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	Name    = "agent-maid"
	Version = "0.1.0"
)

// Dir is the base config directory: $AGENT_MAID_DIR, then $XDG_CONFIG_HOME,
// then %AppData% (Windows) / ~/.config (Linux & macOS).
func Dir() string {
	if d := strings.TrimSpace(os.Getenv("AGENT_MAID_DIR")); d != "" {
		return d
	}
	if base := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); base != "" {
		return filepath.Join(base, Name)
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("AppData"); appData != "" {
			return filepath.Join(appData, Name)
		}
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

// migrateMarker 迁移完成标记文件(位于统一配置目录根)。
// 迁移只做一次:否则每次启动都会把旧目录里存在、新目录里缺失的槽位补回来,
// 用户在新位置删除的账号会被"复活"。
const migrateMarker = ".migrated"

// MigrateLegacy copies data from the two predecessor tools on first run, so
// existing account libraries and slots carry over untouched. Best effort:
// any failure is silently ignored and the new layout starts empty.
//
//	~/.config/gptswitch/store.json            → chatgpt/store.json
//	~/.config/chatgpt-app-account-switch/...  → (same, second candidate)
//	~/.config/zcode-acc-switch/slots/*.json   → zcode/slots/*.json
//
// 只跑一次:写过标记、或新目录已经在用(说明用户开始管理数据了)之后,
// 不再从旧目录补文件——旧目录只是历史快照,以新位置为准。
func MigrateLegacy() {
	marker := filepath.Join(Dir(), migrateMarker)
	if _, err := os.Stat(marker); err == nil {
		return // 已经迁移过
	}

	// 新目录已存在 = 用户已在 agent-maid 里保存/管理过数据。
	// 兼容在修复前用过本程序的用户:不补迁移,直接视为完成。
	slotsDst := filepath.Join(ZcodeBaseDir(), "slots")
	gptDst := GPTStorePath()
	if dirExists(slotsDst) || dirExists(filepath.Dir(gptDst)) {
		writeMarker(marker)
		return
	}

	if os.Getenv("ZCODE_ACC_SWITCH_DIR") == "" {
		// 用户显式指定了槽位目录时完全沿用,不做迁移
		migrateDir(legacyConfigDir("zcode-acc-switch"), slotsDst)
	}
	for _, old := range []string{
		filepath.Join(legacyConfigDir("gptswitch"), "store.json"),
		filepath.Join(legacyConfigDir("chatgpt-app-account-switch"), "store.json"),
	} {
		if migrateFile(old, gptDst) {
			break
		}
	}
	writeMarker(marker)
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func writeMarker(marker string) {
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(marker, []byte("1\n"), 0o600)
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
