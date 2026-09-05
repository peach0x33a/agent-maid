// 共享 lipgloss 样式与跨工具的消息。
package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).
			MarginBottom(1)
	styleDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleOk   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleErr  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styleWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleSel  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	styleHelp = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1)
	styleBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 2)

	// styleInUseName 使用中条目的账号名(绿色加粗,与徽章呼应)。
	styleInUseName = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	// stylePlainName 普通条目的账号名。
	stylePlainName = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
)

// badgeInUse "使用中"徽章(绿底深字,与底部操作栏选中态同款)。
func badgeInUse() string {
	return lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("#15151E")).Background(lipgloss.Color("42")).
		Render(" 使用中 ")
}

// backMsg 从目标管理器返回首屏选择。
type backMsg struct{}

func backCmd() tea.Msg { return backMsg{} }
