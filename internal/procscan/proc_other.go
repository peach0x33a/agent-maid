//go:build !linux && !darwin && !windows

package procscan

// 其他平台无进程枚举实现。
func candidates() []candidate { return nil }
