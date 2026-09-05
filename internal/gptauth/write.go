package gptauth

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"agent-maid/internal/atomicio"
)

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

// WriteAuthFile replaces auth.json atomically with 0600 permissions.
func WriteAuthFile(home string, data []byte) (changed bool, err error) {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return false, err
	}
	path := AuthFilePath(home)
	existing, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
		existing = nil
	}
	if existing != nil && bytes.Equal(bytes.TrimSpace(existing), bytes.TrimSpace(data)) {
		return false, nil
	}
	if err := atomicio.WriteFile(path, data, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// ClearAuthFile 删除 auth.json(仅本地登出,不吊销服务端令牌);
// 文件不存在时静默。
func ClearAuthFile(home string) error {
	err := os.Remove(AuthFilePath(home))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
