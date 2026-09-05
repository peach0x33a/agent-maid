//go:build darwin

package procscan

import (
	"os/exec"
	"strconv"
	"strings"
)

// candidates 通过 `ps -axo pid=,command=` 枚举进程。
// 输出每行为 "PID 命令及参数";argv 取空格分词,属尽力还原
// (含空格的参数会被拆开,对按可执行名匹配的场景足够)。
func candidates() []candidate {
	out, err := exec.Command("ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return nil
	}
	var cands []candidate
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pidStr, cmd, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
		if err != nil {
			continue
		}
		argv := strings.Fields(cmd)
		if len(argv) == 0 {
			continue
		}
		cands = append(cands, candidate{pid: pid, cmd: cmd, argv: argv})
	}
	return cands
}
