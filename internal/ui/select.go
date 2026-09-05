package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"agent-maid/internal/meta"
)

// targetSelectedMsg 首屏选择完成,携带目标。
type targetSelectedMsg struct{ target Target }

// SelectModel 首屏:选择要管理的目标软件。
//
// 布局:单个外框装下两栏(无卡片接缝),随终端宽度自适应——
// 宽终端左右两栏大图,中等宽度换小图,窄终端上下两栏;
// 整体在终端内水平垂直居中。未选中的一栏整体置灰。
type SelectModel struct {
	cursor int
	width  int
	height int
}

func NewSelect() SelectModel { return SelectModel{} }

func (m SelectModel) Init() tea.Cmd { return nil }

func (m SelectModel) Update(msg tea.Msg) (SelectModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "left", "right", "h", "l", "tab", "up", "down", "j", "k", "shift+tab":
			m.cursor = 1 - m.cursor // 两个目标,方向键即切换
		case "1":
			m.cursor = 0
		case "2":
			m.cursor = 1
		case "enter":
			return m, func() tea.Msg { return targetSelectedMsg{Target(m.cursor + 1)} }
		}
	}
	return m, nil
}

// SelectedTarget 当前光标对应的目标(供 main 预选后展示)。
func (m SelectModel) SelectedTarget() Target { return Target(m.cursor + 1) }

func (m SelectModel) View() string {
	w, h := m.width, m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}

	// 尺寸自适应:能横排就横排,并尽量用大图
	small := w < 70
	cols := m.columns(small)
	gap := "     "
	inner := lipgloss.JoinHorizontal(lipgloss.Top, cols[0], gap, cols[1])
	if w < 52 {
		// 窄终端:上下两栏
		inner = lipgloss.JoinVertical(lipgloss.Center, cols[0], "", cols[1])
	}

	// 外框:随终端宽度自适应,内容在框内居中,框在终端内居中
	contentW := lipgloss.Width(inner)
	boxW := contentW
	if w-4 > boxW {
		boxW = min(w-4, 96)
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 3).
		Width(boxW).
		Align(lipgloss.Center).
		Render(inner)

	block := lipgloss.JoinVertical(lipgloss.Center,
		styleTitle.Render("agent-maid · 账号管家"),
		styleDim.Render("选择要管理的目标软件"),
		"",
		box,
		"",
		styleHelp.Render("←/→ 或 Tab 选择 · 1/2 直接选 · Enter 进入 · q 退出"),
		styleDim.Render(meta.Name+" v"+meta.Version),
	)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, block)
}

// columns 渲染两栏内容。选中的一栏:品牌色图标 + 高亮名称;
// 未选中一栏整体置灰。
func (m SelectModel) columns(small bool) [2]string {
	var out [2]string
	for i := range targets {
		t := targets[i]
		sel := i == m.cursor

		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(t.accent)
		textStyle := styleDim
		if !sel {
			nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244"))
		}

		// 图标作为整块参与排版(渐变半块方块,各行为等宽像素行)
		artBlock := t.renderArt(small, !sel)
		artW := lipgloss.Width(artBlock)
		name := t.name
		if sel {
			name = "❯ " + name
		}
		lines := []string{
			nameStyle.Render(name),
			textStyle.Render(t.desc),
			textStyle.Render(t.path),
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
		out[i] = joinLines(rendered)
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
