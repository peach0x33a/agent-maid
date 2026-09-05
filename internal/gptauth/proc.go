// Codex 进程检测:Codex 运行中时切换凭据可能被其回写覆盖,
// 界面据此提示用户重启。
package gptauth

import (
	"path/filepath"
	"strings"

	"agent-maid/internal/procscan"
)

// CodexProcs 返回运行中的 Codex 相关进程(排除本工具自身)。
func CodexProcs() []procscan.Proc {
	return procscan.Find(isCodexProc)
}

// CodexRunning 是否有 Codex 进程在运行。
func CodexRunning() bool { return len(CodexProcs()) > 0 }

// isCodexProc 根据 argv 判断是否 Codex 相关进程。
func isCodexProc(argv []string) bool {
	for _, a := range argv {
		if strings.Contains(a, "agent-maid") {
			return false // 本工具自身
		}
	}
	base := strings.ToLower(filepath.Base(argv[0]))
	if base == "codex" {
		return true
	}
	return strings.HasPrefix(base, "codex-")
}
