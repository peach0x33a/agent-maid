// Antigravity 进程检测:agy CLI 或 Antigravity Desktop 运行中时切换凭据
// 可能被其退出前/续期时的凭据回写覆盖,界面据此提示/等待。
package antigravity

import (
	"path/filepath"
	"strings"

	"agent-maid/internal/procscan"
)

// Proc 一个检测到的 Antigravity 相关进程。
type Proc = procscan.Proc

// Procs 返回运行中的 Antigravity(agy CLI / Desktop)相关进程(排除本工具自身)。
func Procs() []Proc {
	return procscan.Find(isAntigravityProc)
}

// isAntigravityProc 按 argv 判断是否 Antigravity 相关进程:
// - agy CLI: agy / antigravity-cli
// - Antigravity Desktop: Antigravity / Antigravity Helper*
// - 共用的 language_server(--override_ide_name antigravity / --app_data_dir antigravity)
func isAntigravityProc(argv []string) bool {
	for _, a := range argv {
		if strings.Contains(a, "agent-maid") {
			return false // 本工具自身
		}
	}
	if len(argv) == 0 {
		return false
	}
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(argv[0])), ".exe")
	switch base {
	case "agy", "antigravity", "antigravity-cli":
		return true
	case "language_server":
		for _, a := range argv[1:] {
			if strings.EqualFold(a, "antigravity") {
				return true
			}
		}
		return false
	}
	// 1.5.13 起 agy 可能带 bundled agy-node / jetski 语言服务子进程;
	// 它们运行期间同样会回写凭据。
	switch base {
	case "agy-node", "jetski-language-server":
		return true
	}
	// 辅助进程/桌面端 Helper(如 antigravity-cli/bin/agentapi, Antigravity Helper)
	if base == "agentapi" && strings.Contains(strings.ToLower(argv[0]), "antigravity") {
		return true
	}
	return strings.HasPrefix(base, "antigravity-cli") ||
		strings.HasPrefix(base, "antigravity-") ||
		strings.HasPrefix(base, "antigravity ")
}

// Running 是否有 Antigravity 进程(agy CLI 或 Desktop)在运行。
func Running() bool { return len(Procs()) > 0 }
