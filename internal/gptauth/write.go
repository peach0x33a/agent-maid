package gptauth

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent-maid/internal/atomicio"
)

const backupKeep = 10

// ResolveHome determines the official Codex data directory: explicit flag,
// then $CODEX_HOME, then ~/.codex.
func ResolveHome(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Abs(explicit)
	}
	if env := strings.TrimSpace(os.Getenv("CODEX_HOME")); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法确定用户主目录: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

// AuthFilePath is the credential file the official binary reads at startup.
func AuthFilePath(home string) string {
	return filepath.Join(home, "auth.json")
}

// ReadAuthFile returns the current auth.json bytes, or nil when absent.
func ReadAuthFile(home string) ([]byte, error) {
	data, err := os.ReadFile(AuthFilePath(home))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// WriteAuthFile replaces auth.json atomically with 0600 permissions. When the
// file exists and its content changes, the previous bytes are kept as
// auth.json.bak-<timestamp> (newest backupKeep files retained) so a bad
// switch can always be undone by hand.
func WriteAuthFile(home string, data []byte) (backup string, changed bool, err error) {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return "", false, err
	}
	path := AuthFilePath(home)
	existing, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", false, err
		}
		existing = nil
	}
	if existing != nil && bytes.Equal(bytes.TrimSpace(existing), bytes.TrimSpace(data)) {
		return "", false, nil
	}
	if len(existing) > 0 {
		backup = filepath.Join(home, "auth.json.bak-"+time.Now().Format("20060102-150405.000"))
		if err := atomicio.WriteFile(backup, existing, 0o600); err != nil {
			return "", false, fmt.Errorf("备份原 auth.json 失败: %w", err)
		}
		pruneBackups(home)
	}
	if err := atomicio.WriteFile(path, data, 0o600); err != nil {
		return "", false, err
	}
	return backup, true, nil
}

func pruneBackups(home string) {
	matches, err := filepath.Glob(filepath.Join(home, "auth.json.bak-*"))
	if err != nil || len(matches) <= backupKeep {
		return
	}
	sort.Strings(matches)
	for _, stale := range matches[:len(matches)-backupKeep] {
		os.Remove(stale)
	}
}
