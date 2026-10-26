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
	out, _ := tableLayout(rows, nil)
	return out
}

// tableWidth 表格可用宽度(整页宽度留出边距)。
func (b baseModel) tableWidth() int {
	w := b.width
	if w <= 0 {
		w = 80
	}
	tw := w - 2
	if tw < 20 {
		tw = 20
	}
	return tw
}

// tableLayout 按被丢弃的列集合渲染表格,返回结果与各列宽度。
func tableLayout(rows [][]string, dropped map[int]bool) (string, []int) {
	if len(rows) == 0 {
		return "", nil
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
			if dropped[i] {
				continue
			}
			if w := lipgloss.Width(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	var out []string
	for _, r := range rows {
		var parts []string
		lastKept := -1
		for i := range r {
			if !dropped[i] {
				lastKept = i
			}
		}
		for i, cell := range r {
			if dropped[i] {
				continue
			}
			if i == lastKept {
				parts = append(parts, cell)
				continue
			}
			parts = append(parts, lipgloss.NewStyle().Width(widths[i]).Render(cell))
		}
		out = append(out, strings.TrimRight(strings.Join(parts, "  "), " "))
	}
	return strings.Join(out, "\n"), widths
}

// fitTable 在窄终端下按 dropOrder 依次丢弃次要列,直到表格宽度不超过
// width。列宽恒定是表格可读的前提:宁可少显示一列,也不要让单元格折行
// 把整行结构冲散。
func fitTable(rows [][]string, width int, dropOrder []int) string {
	if len(rows) == 0 {
		return ""
	}
	dropped := map[int]bool{}
	out, _ := tableLayout(rows, dropped)
	if width <= 0 || lipgloss.Width(out) <= width {
		return out
	}
	for _, d := range dropOrder {
		dropped[d] = true
		// 全丢光之前一直尝试:至少要留下主标识列与状态列
		if len(dropped) >= len(rows[0])-1 {
			break
		}
		out, _ = tableLayout(rows, dropped)
		if lipgloss.Width(out) <= width {
			return out
		}
	}
	out, _ = tableLayout(rows, dropped)
	return out
}
