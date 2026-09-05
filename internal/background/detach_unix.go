//go:build !windows

package background

import (
	"os/exec"
	"syscall"
)

// detach 让子进程脱离当前终端会话(新会话),终端关闭后仍继续运行。
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
