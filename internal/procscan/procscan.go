// Package procscan 跨平台进程扫描:按调用方提供的 argv 匹配器查找目标
// 进程,自动排除本工具(agent-maid)自身。
//
// 各平台后端:
//   - Linux  — 直接读 /proc,argv 完整;
//   - macOS  — `ps -axo pid=,command=`,argv 为输出的空格分词(尽力而为);
//   - Windows — `tasklist /fo csv /nh`,仅有映像名(.exe 后缀已去除)。
package procscan

import "os"

// Proc 一个匹配到的进程。
type Proc struct {
	Pid int
	Cmd string
}

// candidate 后端产出的候选进程。
type candidate struct {
	pid  int
	cmd  string
	argv []string
}

// Find 扫描系统进程,返回匹配器命中的进程。匹配器收到尽力还原的 argv;
// 含 "agent-maid" 字样的 argv 一律排除(本工具自身),本进程 PID 也排除。
// 不支持的平台返回 nil。
func Find(match func(argv []string) bool) []Proc {
	self := os.Getpid()
	var out []Proc
	for _, cand := range candidates() {
		if cand.pid == self || !match(cand.argv) {
			continue
		}
		out = append(out, Proc{Pid: cand.pid, Cmd: cand.cmd})
	}
	return out
}
