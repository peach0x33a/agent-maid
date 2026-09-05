package zcode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// MachineInfo 描述凭据加密密钥绑定到的环境,以及设备标识。
type MachineInfo struct {
	Platform    string `json:"platform"`
	HomeDir     string `json:"homeDir"`
	Username    string `json:"username"`
	SecretSrc   string `json:"secretSource"` // env / fallback
	Fingerprint string `json:"keyFingerprint"`
	DeviceMid   string `json:"deviceMid,omitempty"`
}

// GetMachineInfo 汇总当前机器信息。
func GetMachineInfo() MachineInfo {
	src := "fallback(platform+home+user)"
	if os.Getenv(secretEnvVar) != "" {
		src = "环境变量 ZCODE_CREDENTIAL_SECRET"
	}
	m := MachineInfo{
		Platform:  runtime.GOOS,
		HomeDir:   homeDir(),
		SecretSrc: src,
	}
	if m.Username = os.Getenv("USER"); m.Username == "" {
		m.Username = "unknown"
	}
	secret := CredentialSecret()
	m.Fingerprint = KeyFingerprint(DeriveKey(secret))
	if mid, err := DeviceMid(); err == nil {
		m.DeviceMid = mid
	}
	return m
}

// Proc 一个检测到的 ZCode 相关进程。
type Proc struct {
	Pid int
	Cmd string
}

// ZCodeProcs 返回正在运行的 ZCode 相关进程(排除本工具自身)。
// 覆盖:主进程 ZCode、AppImage(ZCode-*.AppImage)、挂载点内的 zcode 及其
// Electron 子进程(--type=zygote 等)。
func ZCodeProcs() []Proc {
	self := os.Getpid()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []Proc
	for _, e := range entries {
		if !isPid(e.Name()) {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		argv := strings.FieldsFunc(string(cmdline), func(r rune) bool { return r == 0 })
		if len(argv) == 0 || !isZcodeProc(argv) {
			continue
		}
		out = append(out, Proc{Pid: pid, Cmd: strings.Join(argv, " ")})
	}
	return out
}

// isZcodeProc 根据argv判断是否 ZCode 相关进程。
func isZcodeProc(argv []string) bool {
	for _, a := range argv {
		// 本工具自身及其被 shell 引用的路径
		if strings.Contains(a, "agent-maid") {
			return false
		}
	}
	base := strings.ToLower(filepath.Base(argv[0]))
	if base == "zcode" {
		return true // 主进程与挂载点内二进制(/tmp/.mount_*/zcode)
	}
	if strings.HasPrefix(base, "zcode-") {
		return true // ZCode-3.10.2-linux-x64.AppImage 之类
	}
	return false
}

// ZCodeRunning 是否有 ZCode 进程在运行。
func ZCodeRunning() bool { return len(ZCodeProcs()) > 0 }

// WaitZcodeExit 阻塞直到没有 ZCode 进程或 ctx 取消。
func WaitZcodeExit(ctx context.Context, interval time.Duration) error {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if !ZCodeRunning() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func isPid(s string) bool {
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

// EqualCredentials 判断两份凭据内容是否一致(键值完全相同)。
func EqualCredentials(a, b Credentials) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// Describe 返回便于界面显示的一行摘要。
func (m MachineInfo) Describe() string {
	s := fmt.Sprintf("%s | home=%s | user=%s | 密钥来源=%s | 密钥指纹=%s", m.Platform, m.HomeDir, m.Username, m.SecretSrc, m.Fingerprint)
	if m.DeviceMid != "" {
		s += fmt.Sprintf(" | deviceMid=%s", m.DeviceMid)
	}
	return s
}
