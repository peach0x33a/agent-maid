// 进程检测:提示用户 Antigravity 正在运行时切换凭据可能被其回写覆盖。
package antigravity

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Procs 返回运行中的 Antigravity 相关进程(排除本工具自身)。
func Procs() []Proc {
	self := os.Getpid()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []Proc
	for _, e := range entries {
		if !isPid(e.Name()) {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		argv := strings.FieldsFunc(string(cmdline), func(r rune) bool { return r == 0 })
		if len(argv) == 0 || !isAntigravityProc(argv) {
			continue
		}
		out = append(out, Proc{Pid: pid, Cmd: strings.Join(argv, " ")})
	}
	return out
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

func isPid(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
