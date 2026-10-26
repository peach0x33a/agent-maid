// agy 进程检测:agy 运行中时切换凭据可能被其退出前的凭据回写覆盖,
// 界面据此提示/等待。
package antigravity

import (
	"path/filepath"
	"strings"

	"agent-maid/internal/procscan"
)

// Proc 一个检测到的 agy 相关进程。
type Proc = procscan.Proc

// Procs 返回运行中的 agy 相关进程(排除本工具自身)。
func Procs() []Proc {
	return procscan.Find(isAntigravityProc)
}

// isAntigravityProc 按 argv 判断是否 agy 相关进程:主程序名为 agy,
// 部分安装是 antigravity / antigravity-cli 的软链或副本。
func isAntigravityProc(argv []string) bool {
	for _, a := range argv {
		if strings.Contains(a, "agent-maid") {
			return false // 本工具自身
		}
	}
	if len(argv) == 0 {
		return false
	}
	base := strings.ToLower(filepath.Base(argv[0]))
	switch base {
	case "agy", "antigravity", "antigravity-cli":
		return true
	}
	// agy 的辅助进程/挂载点(antigravity-cli/bin/agentapi 之类)
	return strings.HasPrefix(base, "antigravity-cli") ||
		strings.HasPrefix(base, "antigravity-")
}

// Running 是否有 agy 进程在运行。
func Running() bool { return len(Procs()) > 0 }
