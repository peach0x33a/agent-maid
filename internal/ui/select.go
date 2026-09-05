package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"agent-maid/internal/meta"
)

// artTickMsg 首屏流光动画的节拍。
type artTickMsg struct{}

func artTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return artTickMsg{} })
}

// SelectModel 首屏:选择要管理的目标软件。
//
// 布局:单个外框装下所有工具栏位(无卡片接缝),随终端宽度自适应——
// 宽终端左右并排大图,中等宽度换小图,窄终端上下堆叠;
// 整体在终端内水平垂直居中。未选中的一栏整体置灰,
// 选中的 logo 播放流光动画(渐变相位沿对角线循环流动)。
type SelectModel struct {
	infos  []*targetInfo // 注册表中各工具的展示信息
	cursor int
	width  int
	height int
	phase  int // 流光动画当前帧
}

// NewSelect 从注册表构建首屏。
func NewSelect(reg []Manager) SelectModel {
	infos := make([]*targetInfo, len(reg))
	for i, m := range reg {
		infos[i] = m.Info()
	}
	return SelectModel{infos: infos}
}

func (m SelectModel) Init() tea.Cmd { return artTick() }

func (m SelectModel) Update(msg tea.Msg) (SelectModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case artTickMsg:
		m.phase++
		return m, artTick()
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "left", "right", "h", "l", "tab", "up", "down", "j", "k", "shift+tab":
			if len(m.infos) > 1 {
				if msg.String() == "left" || msg.String() == "h" || msg.String() == "up" || msg.String() == "k" {
					m.cursor = (m.cursor + len(m.infos) - 1) % len(m.infos)
				} else {
					m.cursor = (m.cursor + 1) % len(m.infos)
				}
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if i := int(msg.String()[0] - '1'); i < len(m.infos) {
				m.cursor = i
			}
		case "enter":
			return m, func() tea.Msg { return targetSelectedMsg{Index: m.cursor} }
		}
	}
	return m, nil
}

func (m SelectModel) View() string {
	w, h := m.width, m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}

	// 尺寸自适应:优先大图横排,放不下换小图横排,再放不下纵向堆叠。
	// 可用宽度要扣掉外框边框(2)与左右内边距(6),否则行尾会折行错位。
	const boxChrome = 8
	cols := m.columns(false, m.phase)
	if totalWidth(cols) > w-boxChrome {
		cols = m.columns(true, m.phase)
	}

	var inner string
	switch {
	case len(cols) == 1:
		inner = cols[0]
	case totalWidth(cols) <= w-boxChrome:
		inner = joinHorizontal(cols, "     ")
	default:
		inner = joinVertical(cols, "")
	}

	// 外框:至少容纳内容+内边距(绝不会被折行);宽终端舒展,上限 108
	contentW := lipgloss.Width(inner)
	boxW := contentW + 6
	if maxW := w - 2; boxW > maxW {
		boxW = maxW
	}
	if stretch := w - 6; stretch > boxW {
		boxW = max(boxW, min(stretch, 108))
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 3).
		Width(boxW).
		Align(lipgloss.Center).
		Render(inner)

	hint := "←/→ 或 Tab 选择"
	if n := len(m.infos); n > 1 {
		digits := make([]string, 0, n)
		for i := 1; i <= n; i++ {
			digits = append(digits, fmt.Sprintf("%d", i))
		}
		hint += " · " + strings.Join(digits, "/") + " 直接选"
	}
	hint += " · Enter 进入 · q 退出"

	block := lipgloss.JoinVertical(lipgloss.Center,
		styleTitle.Render("Agent Maid |´・ω・)ノ"),
		styleDim.Render("选择要管理的目标软件"),
		"",
		box,
		"",
		styleHelp.Render(hint),
		styleDim.Render(meta.Name+" v"+meta.Version),
	)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, block)
}

// columns 渲染各工具栏位。选中的一栏:品牌色流光图标 + 高亮名称;
// 未选中一栏整体置灰。
func (m SelectModel) columns(small bool, phase int) []string {
	cols := make([]string, 0, len(m.infos))
	for i := range m.infos {
		info := m.infos[i]
		sel := i == m.cursor

		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(info.accent)
		if !sel {
			nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244"))
		}

		// 图标作为整块参与排版(渐变半块方块,各行为等宽像素行)
		artBlock := info.renderArt(small, !sel, phase)
		artW := lipgloss.Width(artBlock)
		name := info.Name
		if sel {
			name = "❯ " + name
		}
		lines := []string{
			nameStyle.Render(name),
			styleDim.Render(info.Desc),
			styleDim.Render(info.Path),
		}
		colW := artW
		for _, line := range lines {
			if lw := lipgloss.Width(line); lw > colW {
				colW = lw
			}
		}

		var rendered []string
		for _, line := range splitLines(artBlock) {
			rendered = append(rendered, lipgloss.PlaceHorizontal(colW, lipgloss.Center, line))
		}
		rendered = append(rendered, "")
		for _, line := range lines {
			rendered = append(rendered, lipgloss.PlaceHorizontal(colW, lipgloss.Center, line))
		}
		cols = append(cols, joinLines(rendered))
	}
	return cols
}

func joinHorizontal(cols []string, gap string) string {
	switch len(cols) {
	case 0:
		return ""
	case 1:
		return cols[0]
	}
	args := make([]string, 0, len(cols)*2-1)
	args = append(args, cols[0])
	for _, c := range cols[1:] {
		args = append(args, gap, c)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, args...)
}

func joinVertical(cols []string, gap string) string {
	switch len(cols) {
	case 0:
		return ""
	case 1:
		return cols[0]
	}
	args := make([]string, 0, len(cols)*2-1)
	args = append(args, cols[0])
	for _, c := range cols[1:] {
		args = append(args, gap, c)
	}
	return lipgloss.JoinVertical(lipgloss.Center, args...)
}

// totalWidth 计算横排总宽度(含栏间距 5)。
func totalWidth(cols []string) int {
	n := 0
	for i, c := range cols {
		if i > 0 {
			n += 5
		}
		n += lipgloss.Width(c)
	}
	return n
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
