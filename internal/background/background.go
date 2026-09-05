// Package background 派生脱离终端的后台进程:用户关闭 TUI 后,
// 由后台进程继续等待目标软件退出并替换凭据,完成后发系统通知。
package background

import (
	"os"
	"os/exec"
)

// SpawnWaitZcode 派生后台进程,等待 ZCode 完全退出后切换到指定槽位。
// relaunchPath 非空时,替换完成后自动重新启动 ZCode。
func SpawnWaitZcode(slot, relaunchPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"--wait-apply", "zcode", slot}
	if relaunchPath != "" {
		args = append(args, relaunchPath)
	}
	cmd := exec.Command(exe, args...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Launch 以脱离会话的方式启动外部程序(用于替换完成后自动重启软件)。
func Launch(path string, args ...string) error {
	cmd := exec.Command(path, args...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
