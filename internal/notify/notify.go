// 系统级桌面通知:Linux 用 notify-send,macOS 用 osascript。
// 发送失败一律静默(通知是锦上添花,不应影响主流程)。
//go:build !windows

package notify

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Show 发送一条桌面通知。
func Show(title, message string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("notify-send", title, message).Run()
	case "darwin":
		script := fmt.Sprintf("display notification %q with title %q", message, title)
		return exec.Command("osascript", "-e", script).Run()
	default:
		return nil
	}
}
