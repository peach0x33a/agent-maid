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

// NewSelect 从注册表构建首屏。last 为上次使用的工具 Key,光标默认停在它上面。
func NewSelect(reg []Manager, last string) SelectModel {
	infos := make([]*targetInfo, len(reg))
	cursor := 0
	for i, m := range reg {
		infos[i] = m.Info()
		if last != "" && infos[i].Key == last {
			cursor = i
		}
	}
	return SelectModel{infos: infos, cursor: cursor}
}

func (m SelectModel) Init() tea.Cmd {
	if noAnim {
		return nil
	}
	return artTick()
}

func (m SelectModel) Update(msg tea.Msg) (SelectModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case artTickMsg:
		if noAnim {
			return m, nil
		}
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
		case "s", "S":
			return m, func() tea.Msg { return settingsOpenMsg{} }
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

	// 纯文本模式:垂直居中菜单(无图标,一行一个工具,三列对齐)
	var inner string
	if plainMode {
		inner = m.plainMenu()
	} else {
		// 轮播布局:选中项恒居中(大图),两侧是上一项/下一项(小图,缩小 20%,
		// 置灰),列表无限循环;窄终端放不下时只显示选中项。
		const boxChrome = 8
		n := len(m.infos)
		center := m.slot(m.cursor, false, false, m.phase)
		if n >= 2 {
			left := m.slot((m.cursor+n-1)%n, true, true, 0)
			right := m.slot((m.cursor+1)%n, true, true, 0)
			row := joinHorizontal([]string{left, center, right}, "     ")
			if totalWidth([]string{left, center, right}) <= w-boxChrome {
				inner = row
			} else {
				inner = center
			}
		} else {
			inner = center
		}
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
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 3).
		Width(boxW)
	if !plainMode {
		boxStyle = boxStyle.Align(lipgloss.Center) // 纯文本菜单各行宽度不同,逐行居中会错位
	}
	box := boxStyle.Render(inner)

	hint := "←/→ 或 Tab 选择"
	if n := len(m.infos); n > 1 {
		digits := make([]string, 0, n)
		for i := 1; i <= n; i++ {
			digits = append(digits, fmt.Sprintf("%d", i))
		}
		hint += " · " + strings.Join(digits, "/") + " 直接选"
	}
	hint += " · Enter 进入 · S 设置"

	title := "Agent Maid |´・ω・)ノ"
	if plainMode {
		title = "Agent Maid"
	}
	block := lipgloss.JoinVertical(lipgloss.Center,
		styleTitle.Render(title),
		styleDim.Render("选择要管理的目标软件"),
		"",
		box,
		"",
		styleHelp.Render(hint),
		styleDim.Render(meta.Name+" v"+meta.Version+" · "+meta.Repo),
	)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, block)
}

// plainMenu 纯文本模式的首屏菜单:一行一个工具,名字/说明/路径三列对齐,
// 整块由外层居中。
func (m SelectModel) plainMenu() string {
	nameW, descW := 0, 0
	for _, info := range m.infos {
		if w := lipgloss.Width(info.Name); w > nameW {
			nameW = w
		}
		if w := lipgloss.Width(info.Desc); w > descW {
			descW = w
		}
	}
	var lines []string
	for i := range m.infos {
		info := m.infos[i]
		sel := i == m.cursor
		cursor := "  "
		nameStyle := stylePlainName
		if sel {
			cursor = styleSel.Bold(true).Render("> ")
			nameStyle = styleSel
		}
		lines = append(lines,
			cursor+nameStyle.Render(lipgloss.NewStyle().Width(nameW).Render(info.Name))+"  "+
				styleDim.Render(lipgloss.NewStyle().Width(descW).Render(info.Desc))+"  "+
				styleDim.Render(info.Path))
	}
	return strings.Join(lines, "\n")
}

// slot 渲染单个工具栏位。dim=true 为两侧项:置灰、小图、仅显示名称;
// 选中项:品牌色流光图标 + 夹角标名称 + 说明/路径。
func (m SelectModel) slot(idx int, small, dim bool, phase int) string {
	info := m.infos[idx]

	artBlock := info.renderArt(small, dim, phase)
	artW := lipgloss.Width(artBlock)

	name := info.Name
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(info.accent)
	if dim {
		nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244"))
	} else if plainMode {
		name = "> " + name + " <"
	} else {
		name = "❯ " + name + " ❮"
	}

	lines := []string{nameStyle.Render(name)}
	if !dim {
		lines = append(lines,
			styleDim.Render(info.Desc),
			styleDim.Render(info.Path))
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
	return joinLines(rendered)
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
