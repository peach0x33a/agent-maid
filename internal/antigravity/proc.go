// 进程检测:提示用户 Antigravity 正在运行时切换凭据可能被其回写覆盖。
package antigravity

import (
	"path/filepath"
	"strings"

	"agent-maid/internal/procscan"
)

// Proc 一个检测到的 Antigravity 相关进程。
type Proc = procscan.Proc

// Procs 返回运行中的 Antigravity 相关进程(排除本工具自身)。
func Procs() []Proc {
	return procscan.Find(isAntigravityProc)
}

// isAntigravityProc 根据 argv 判断是否 Antigravity 相关进程。
func isAntigravityProc(argv []string) bool {
	for _, a := range argv {
		if strings.Contains(a, "agent-maid") {
			return false // 本工具自身
		}
	}
	base := strings.ToLower(filepath.Base(argv[0]))
	if base == "antigravity" {
		return true
	}
	return strings.HasPrefix(base, "antigravity-")
}

// Running 是否有 Antigravity 进程在运行。
func Running() bool { return len(Procs()) > 0 }
