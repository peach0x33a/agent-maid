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

// LaunchCmd 从运行中的进程里推断适合重新启动 Antigravity 的命令
// (Electron 主进程没有 --type= 参数);未检测到时返回空串。
func LaunchCmd() string {
	for _, p := range Procs() {
		argv := strings.Fields(p.Cmd)
		main := true
		for _, a := range argv[1:] {
			if strings.HasPrefix(a, "--type=") {
				main = false
				break
			}
		}
		if main {
			return argv[0]
		}
	}
	return ""
}
