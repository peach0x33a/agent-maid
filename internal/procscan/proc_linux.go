//go:build linux

package procscan

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// candidates 读 /proc 下所有进程的 cmdline。
func candidates() []candidate {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []candidate
	for _, e := range entries {
		if !IsPid(e.Name()) {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		argv := strings.FieldsFunc(string(cmdline), func(r rune) bool { return r == 0 })
		if len(argv) == 0 {
			continue
		}
		out = append(out, candidate{pid: pid, cmd: strings.Join(argv, " "), argv: argv})
	}
	return out
}

// IsPid 判断字符串是否为纯数字 PID。
func IsPid(s string) bool {
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
