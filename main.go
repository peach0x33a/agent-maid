// agent-maid:ChatGPT(Codex)与 ZCode 的账号切换管家。
//
// 用法:
//
//	agent-maid              # 进入 TUI,首屏选择目标软件
//	agent-maid chatgpt      # 直接进入 ChatGPT 账号管理
//	agent-maid zcode        # 直接进入 ZCode 账号管理
//	agent-maid antigravity  # 直接进入 Antigravity 账号管理
//	agent-maid version      # 显示版本
//
// 隐藏子命令(由 TUI 派生,不面向用户):
//
//	agent-maid --wait-apply zcode <槽位名>
//	    后台等待 ZCode 完全退出后切换到指定槽位,并发系统通知。
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"agent-maid/internal/antigravity"
	"agent-maid/internal/background"
	"agent-maid/internal/meta"
	"agent-maid/internal/notify"
	"agent-maid/internal/ui"
	"agent-maid/internal/zcode"
	"agent-maid/internal/zcodeapp"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--wait-apply" {
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "用法: agent-maid --wait-apply <zcode> <槽位名> [重启路径]")
			os.Exit(2)
		}
		relaunch := ""
		if len(os.Args) > 4 {
			relaunch = os.Args[4]
		}
		if err := runWaitApply(os.Args[2], os.Args[3], relaunch); err != nil {
			_ = notify.Show("Agent Maid", "后台切换失败: "+err.Error())
			fmt.Fprintf(os.Stderr, "后台切换失败: %v\n", err)
			os.Exit(1)
		}
		return
	}

	pre := ""
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "chatgpt", "gpt":
			pre = "chatgpt"
		case "zcode", "z":
			pre = "zcode"
		case "antigravity", "anti", "ag":
			pre = "antigravity"
		case "version", "--version", "-v":
			fmt.Printf("%s %s\n", meta.Name, meta.Version)
			return
		default:
			fmt.Fprintf(os.Stderr, "未知参数 %q\n可用: %s | version\n",
				os.Args[1], strings.Join(ui.TargetKeys(), " | "))
			os.Exit(2)
		}
	}

	// 首次运行时把两个前身工具的数据搬到统一目录(不覆盖已有文件)
	meta.MigrateLegacy()

	p := tea.NewProgram(ui.NewRoot(pre), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI 运行失败: %v\n", err)
		os.Exit(1)
	}
}

// runWaitApply 等待目标软件完全退出后切换槽位,并发系统通知。
func runWaitApply(target, slot, relaunch string) error {
	switch target {
	case "zcode":
		if err := zcode.WaitZcodeExit(context.Background(), 500*time.Millisecond); err != nil {
			return err
		}
		backup, err := zcodeapp.New().Apply(slot)
		if err != nil {
			return err
		}
		msg := "ZCode 已退出,已切换到槽位 " + slot
		if backup != "" {
			msg += "(原账号已自动备份到 " + backup + ")"
		}
		_ = notify.Show("Agent Maid", msg)
		return relaunchAfter(relaunch)
	case "antigravity":
		for antigravity.Running() {
			time.Sleep(500 * time.Millisecond)
		}
		backup, err := antigravity.New().Apply(slot)
		if err != nil {
			return err
		}
		msg := "Antigravity 已退出,已切换到槽位 " + slot
		if backup != "" {
			msg += "(原账号已自动备份到 " + backup + ")"
		}
		_ = notify.Show("Agent Maid", msg)
		return relaunchAfter(relaunch)
	default:
		return fmt.Errorf("未知目标 %q", target)
	}
}

// relaunchAfter 替换完成后按需重启目标软件。
func relaunchAfter(path string) error {
	if path == "" {
		return nil
	}
	return background.Launch(path)
}
