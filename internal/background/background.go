// Package background 派生脱离终端的后台进程:用户关闭 TUI 后,
// 由后台进程继续等待目标软件退出并替换凭据,完成后发系统通知。
package background

import (
	"os"
	"os/exec"
)

// SpawnWaitZcode 派生后台进程,等待 ZCode 完全退出后切换到指定槽位。
func SpawnWaitZcode(slot string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--wait-apply", "zcode", slot)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
