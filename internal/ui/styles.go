// Package ui 实现 agent-maid 的 bubbletea TUI:首屏选择目标软件,
// 再分别进入 ChatGPT 账号切换器与 ZCode 账号切换器。
package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"agent-maid/internal/zcode"
)

// ───────────────────────── 共享样式 ─────────────────────────

var (
	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).
			MarginBottom(1)
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleOk     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styleWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleSel    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	styleHelp   = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1)
	styleBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 2)
	styleHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).
			Border(lipgloss.NormalBorder(), false, false, true, false).MarginBottom(0)
)

// ───────────────────────── 共享模式/状态 ─────────────────────────

// mode 两个目标管理器共用的界面模式。
type mode int

const (
	modeNormal  mode = iota
	modeInput        // 输入框(ZCode: 保存/重命名;ChatGPT: 导入)
	modeConfirm      // 确认操作
	modeWaiting      // ZCode 专用:等待 ZCode 退出后执行切换
	modeHelp
	modeMachine // ZCode 专用:机器信息
	modeDetail  // ChatGPT 专用:账号详情
)

// inputPurpose 输入框用途。
type inputPurpose int

const (
	inputSave    inputPurpose = iota // ZCode:保存当前账号
	inputRename                      // ZCode:重命名槽位
	inputImport                      // ChatGPT:导入 JSON 文件路径或粘贴内容
	inputGSave                       // ChatGPT:保存当前凭据(输入显示名)
	inputGRename                     // ChatGPT:重命名账号显示名
)

// chip 底部操作栏的一个操作项(可直接按 key 触发,也可左右键选中后回车)。
type chip struct {
	key   string
	label string
}

// chipBar 渲染操作栏,sel 为选中项(高亮),accent 为平台强调色。
func chipBar(chips []chip, sel int, accent lipgloss.Color) string {
	parts := make([]string, 0, len(chips))
	for i, c := range chips {
		if i == sel {
			st := lipgloss.NewStyle().Bold(true).
				Foreground(lipgloss.Color("#15151E")).Background(accent)
			parts = append(parts, st.Render(" "+c.key+" "+c.label+" "))
		} else {
			parts = append(parts, styleDim.Render(" "+c.key+" "+c.label+" "))
		}
	}
	return strings.Join(parts, " ")
}

// banner 子界面顶部持续显示的标题行。
func banner(t Target) string {
	name := ""
	switch t {
	case TargetChatGPT:
		name = "ChatGPT账号管理"
	case TargetZCode:
		name = "ZCode账号管理"
	}
	return styleHeader.Render(lipgloss.NewStyle().Bold(true).Render("AgentMaid - " + name))
}

// status 底部状态行。
type status struct {
	kind int // 0 info 1 ok 2 err 3 warn
	text string
}

func (s status) render() string {
	switch s.kind {
	case 1:
		return styleOk.Render("✔ " + s.text)
	case 2:
		return styleErr.Render("✘ " + s.text)
	case 3:
		return styleWarn.Render("⚠ " + s.text)
	case 4:
		return styleErr.Render(s.text)
	default:
		return styleDim.Render(s.text)
	}
}

// backMsg 从目标管理器返回首屏选择。
type backMsg struct{}

func backCmd() tea.Msg { return backMsg{} }

// zcodeAccountLabel 渲染 ZCode 账号摘要(provider · 用户名)。
func zcodeAccountLabel(meta zcode.AccountMeta) string {
	parts := []string{}
	if meta.Provider != "" {
		parts = append(parts, meta.Provider)
	}
	name := meta.DisplayName
	if name == "" {
		name = meta.Username
	}
	if name != "" {
		parts = append(parts, name)
	}
	if len(parts) == 0 {
		return "未知账号(密钥不匹配或凭据为空)"
	}
	return strings.Join(parts, " · ")
}
