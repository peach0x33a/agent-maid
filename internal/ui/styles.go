// 共享 lipgloss 样式与跨工具的消息。
package ui

import (
	"strings"
	"time"

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

// backMsg 从目标管理器返回首屏选择。
type backMsg struct{}

func backCmd() tea.Msg { return backMsg{} }

// shortTime 列表用的短时间显示(无记录返回 —)。
func shortTime(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.Local().Format("01-02 15:04")
}

// shortID8 取 ID 前 8 位。
func shortID8(id string) string {
	if id == "" {
		return "—"
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// joinTable 把已样式化的单元格行渲染为列宽对齐的表格:
// 每列宽度取该列最大显示宽度,最后一列不填充(内容可变长)。
func joinTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	for _, r := range rows {
		for i, cell := range r {
			if w := lipgloss.Width(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	var out []string
	for _, r := range rows {
		parts := make([]string, 0, len(r))
		for i, cell := range r {
			if i == len(r)-1 {
				parts = append(parts, cell)
				continue
			}
			parts = append(parts, lipgloss.NewStyle().Width(widths[i]).Render(cell))
		}
		out = append(out, strings.TrimSpace(strings.Join(parts, "  ")))
	}
	return strings.Join(out, "\n")
}
