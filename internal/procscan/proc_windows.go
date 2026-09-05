//go:build windows

package procscan

import (
	"bytes"
	"encoding/csv"
	"os/exec"
	"path/filepath"
	"strconv"
)

// candidates 通过 `tasklist /fo csv /nh` 枚举进程。
// 该输出只有映像名(如 codex.exe)与 PID,没有完整命令行,
// 因此 argv 仅为映像名(已去除 .exe 后缀)。
func candidates() []candidate {
	out, err := exec.Command("tasklist", "/fo", "csv", "/nh").Output()
	if err != nil {
		return nil
	}
	records, err := csv.NewReader(bytes.NewReader(out)).ReadAll()
	if err != nil {
		return nil
	}
	var cands []candidate
	for _, rec := range records {
		if len(rec) < 2 {
			continue
		}
		image := rec[0]
		pid, err := strconv.Atoi(rec[1])
		if err != nil {
			continue
		}
		name := image
		if ext := filepath.Ext(name); ext != "" {
			name = name[:len(name)-len(ext)]
		}
		argv := []string{name}
		cands = append(cands, candidate{pid: pid, cmd: image, argv: argv})
	}
	return cands
}
