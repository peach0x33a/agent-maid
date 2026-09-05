// agent-maid:ChatGPT(Codex)与 ZCode 的账号切换管家。
//
// 用法:
//
//	agent-maid              # 进入 TUI,首屏选择目标软件
//	agent-maid chatgpt      # 直接进入 ChatGPT 账号切换器
//	agent-maid zcode        # 直接进入 ZCode 账号切换器
//	agent-maid version      # 显示版本
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"agent-maid/internal/meta"
	"agent-maid/internal/ui"
)

func main() {
	pre := ui.TargetNone
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "chatgpt", "gpt":
			pre = ui.TargetChatGPT
		case "zcode", "z":
			pre = ui.TargetZCode
		case "version", "--version", "-v":
			fmt.Printf("%s %s\n", meta.Name, meta.Version)
			return
		default:
			fmt.Fprintf(os.Stderr, "未知参数 %q\n可用: chatgpt | zcode | version\n", os.Args[1])
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
