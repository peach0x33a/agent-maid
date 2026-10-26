// agent-maid:ChatGPT(Codex)、ZCode 与 Antigravity 的账号切换管家。
//
// 用法:
//
//	agent-maid              # 进入 TUI,首屏选择目标软件
//	agent-maid chatgpt      # 直接进入 ChatGPT 账号管理
//	agent-maid zcode        # 直接进入 ZCode 账号管理
//	agent-maid antigravity  # 直接进入 Antigravity 账号管理
//	agent-maid version      # 显示版本
//	agent-maid --help       # 显示用法
//
// 无头子命令(给 AI agent / 脚本用,不需要交互终端):
//
//	agent-maid list|current|use|save|rm|import|logout <目标> [...]
//	    见 agent-maid help 的输出;每个命令都支持 --json。
//
// 隐藏子命令(由 TUI 派生,不面向用户):
//
//	agent-maid --wait-apply zcode|antigravity <槽位名>
//	    后台等待目标软件完全退出后切换到指定槽位,并发系统通知。
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"

	"agent-maid/internal/antigravity"
	"agent-maid/internal/cli"
	"agent-maid/internal/meta"
	"agent-maid/internal/notify"
	"agent-maid/internal/ui"
	"agent-maid/internal/zcode"
	"agent-maid/internal/zcodeapp"
)

func main() {
	args := os.Args[1:]

	// 隐藏子命令:由 TUI 派生,不面向用户
	if len(args) > 0 && args[0] == "--wait-apply" {
		if len(args) != 3 {
			fmt.Fprintln(os.Stderr, "用法: agent-maid --wait-apply <zcode|antigravity> <槽位名>")
			os.Exit(2)
		}
		if err := runWaitApply(args[1], args[2]); err != nil {
			_ = notify.Show("Agent Maid", "后台切换失败: "+err.Error())
			fmt.Fprintf(os.Stderr, "后台切换失败: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 无头子命令:不进入 TUI,也不需要交互终端
	if cli.IsCommand(args) {
		meta.MigrateLegacy()
		os.Exit(cli.Run(args, cli.Env{
			Stdout: os.Stdout,
			Stderr: os.Stderr,
			Stdin:  os.Stdin,
		}))
	}

	pre := ""
	if len(args) > 0 {
		// 目标名不区分大小写,方便手输(chatgpt / ChatGPT / CHATGPT 均可)
		switch strings.ToLower(args[0]) {
		case "chatgpt", "gpt":
			pre = "chatgpt"
		case "zcode", "z":
			pre = "zcode"
		case "antigravity", "anti", "ag":
			pre = "antigravity"
		case "version", "--version", "-v":
			if len(args) > 1 {
				failExtraArg(args[1], "version")
			}
			fmt.Printf("%s %s\n", meta.Name, meta.Version)
			return
		case "help", "--help", "-h":
			fmt.Print(usageText())
			fmt.Print(cli.Usage())
			return
		default:
			fmt.Fprintf(os.Stderr, "未知参数 %q\n\n%s%s", args[0], usageText(), cli.Usage())
			os.Exit(2)
		}
		if len(args) > 1 {
			failExtraArg(args[1], args[0])
		}
	}

	// 首次运行时把两个前身工具的数据搬到统一目录(不覆盖已有文件)
	meta.MigrateLegacy()

	if !stdinIsTerminal() {
		fmt.Fprintln(os.Stderr, "错误: agent-maid 需要交互式终端(检测到标准输入不是终端)。")
		fmt.Fprintln(os.Stderr, "若在脚本/管道中使用,请用 agent-maid --help 查看可用的非交互命令。")
		os.Exit(2)
	}

	p := tea.NewProgram(ui.NewRoot(pre), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI 运行失败: %v\n", err)
		fmt.Fprintln(os.Stderr, "提示: 请在真实终端中运行;非交互环境可用 agent-maid --help 查看用法。")
		os.Exit(1)
	}
}

// failExtraArg 报告多余参数并退出(用法错误统一 exit 2)。
func failExtraArg(arg, after string) {
	fmt.Fprintf(os.Stderr, "多余参数 %q(紧跟在 %q 之后)\n\n%s", arg, after, usageText())
	os.Exit(2)
}

// stdinIsTerminal 报告标准输入是否为终端,用于在管道/重定向下给出
// 中文提示而不是 Bubble Tea 的英文报错。(/dev/null 也是字符设备,
// 因此必须用 IsTerminal 而不是 ModeCharDevice 判断。)
func stdinIsTerminal() bool {
	return term.IsTerminal(os.Stdin.Fd())
}

// usageText 帮助文案(仅 CLI 使用,不进入 TUI)。
func usageText() string {
	return "Agent Maid - AI 工具账号管家(TUI)\n\n" +
		"用法:\n" +
		"  agent-maid                 进入 TUI,首屏选择目标软件\n" +
		"  agent-maid <目标>          直接进入对应账号管理\n" +
		"  agent-maid version         显示版本\n" +
		"  agent-maid --help          显示本帮助\n\n" +
		"目标: " + strings.Join(ui.TargetKeys(), " | ") + "(也接受 gpt / z / anti / ag)\n" +
		"目标名不区分大小写。\n\n"
}

// runWaitApply 等待目标软件完全退出后切换槽位,并发系统通知。
func runWaitApply(target, slot string) error {
	switch target {
	case "zcode":
		// 先校验槽位,避免目标退出后才报"槽位不存在"而白等一场
		if _, err := zcodeapp.New().Store.Get(slot); err != nil {
			return fmt.Errorf("读取槽位 %q 失败: %w", slot, err)
		}
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
	case "antigravity":
		// 同上:先确认槽位存在,再进入等待
		if _, err := antigravity.New().Store.Get(slot); err != nil {
			return fmt.Errorf("读取槽位 %q 失败: %w", slot, err)
		}
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
	default:
		return fmt.Errorf("未知目标 %q", target)
	}
	return nil
}
