//go:build windows

package notify

import (
	"syscall"
	"unsafe"
)

var (
	user32      = syscall.NewLazyDLL("user32.dll")
	messageBoxW = user32.NewProc("MessageBoxW")
)

const (
	mbOK            = 0x00000000
	mbSetForeground = 0x00010000
	mbTopmost       = 0x00040000
)

// Show 通过 user32.MessageBoxW 弹出系统级提示(阻塞,按确定关闭)。
func Show(title, message string) error {
	tp, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return err
	}
	mp, err := syscall.UTF16PtrFromString(message)
	if err != nil {
		return err
	}
	messageBoxW.Call(
		0, // HWND_DESKTOP
		uintptr(unsafe.Pointer(mp)),
		uintptr(unsafe.Pointer(tp)),
		mbOK|mbSetForeground|mbTopmost,
	)
	return nil
}
