package zcode

import "testing"

func TestIsZcodeProc(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want bool
	}{
		{"主进程", []string{"ZCode"}, true},
		{"挂载点内二进制(zygote)", []string{"/tmp/.mount_ZCode-0UOeh8/zcode", "--type=zygote", "--no-zygote-sandbox"}, true},
		{"AppImage 启动器", []string{"/home/u/Applications/ZCode-3.10.2-linux-x64.AppImage"}, true},
		{"gpu 进程", []string{"/tmp/.mount_ZCode-x/zcode", "--type=gpu-process"}, true},
		{"本工具自身", []string{"./agent-maid", "apply", "work"}, false},
		{"本工具绝对路径", []string{"/usr/local/bin/agent-maid", "list"}, false},
		{"无关进程", []string{" firefox "}, false},
		{"包含 zcode 字样的无关进程", []string{"not-zcode-helper"}, false},
	}
	for _, c := range cases {
		if got := isZcodeProc(c.argv); got != c.want {
			t.Errorf("%s: isZcodeProc(%v) = %v, 期望 %v", c.name, c.argv, got, c.want)
		}
	}
}
