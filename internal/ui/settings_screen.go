// 设置界面:从首屏按 S 进入。管理界面偏好与向导重跑。
package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"agent-maid/internal/meta"
)

// settingsOpenMsg 首屏请求打开设置。
type settingsOpenMsg struct{}

// reopenWizardMsg 请求重新运行设置向导。
type reopenWizardMsg struct{}

// SettingsModel 设置界面。
type SettingsModel struct {
	cursor int // 0 纯文本模式 1 动画 2 重新运行向导
	width  int
	height int
}

// NewSettings 创建设置界面(读取当前设置)。
func NewSettings() SettingsModel { return SettingsModel{} }

func (m SettingsModel) Init() tea.Cmd { return nil }

func (m SettingsModel) Update(msg tea.Msg) (SettingsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "Q", "esc":
			return m, backCmd
		case "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.cursor = (m.cursor + 3) % 4
		case "down", "j", "tab":
			m.cursor = (m.cursor + 1) % 4
		case "enter", " ":
			s := meta.LoadSettings()
			switch m.cursor {
			case 0:
				s.Plain = !s.Plain
				plainMode = s.Plain
			case 1:
				s.NoAnim = !s.NoAnim
				noAnim = s.NoAnim
			case 2:
				return m, func() tea.Msg { return reopenWizardMsg{} }
			}
			_ = meta.SaveSettings(s)
			return m, nil
		}
	}
	return m, nil
}

func (m SettingsModel) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	items := []string{
		m.item("纯文本模式", toggleCheck(plainMode), m.cursor == 0),
		m.item("动画", toggleCheck(!noAnim), m.cursor == 1),
		m.item("重新运行设置向导", "", m.cursor == 2),
	}

	var b strings.Builder
	b.WriteString(styleTitle.Render("Agent Maid · 设置"))
	b.WriteString("\n\n")
	b.WriteString(strings.Join(items, "\n"))
	b.WriteString("\n\n")
	b.WriteString(styleDim.Render("纯文本模式:不渲染块状图标与特殊符号,适合不支持这些字符的终端。"))
	b.WriteString("\n\n")
	b.WriteString(styleHelp.Render("↑/↓ 选择 · Enter 切换/执行 · Esc 返回"))
	return centerScreen(b.String(), w, m.height)
}

// toggleCheck 定宽复选样式([ x ] / [   ]):纯 ASCII,开/关宽度严格一致。
func toggleCheck(on bool) string {
	if on {
		return styleOk.Bold(true).Render("[x]")
	}
	return styleDim.Render("[ ]")
}

// item 设置条目:文本左对齐,控件(胶囊/数值)右对齐,整体在设置块内对齐。
func (m SettingsModel) item(name, value string, sel bool) string {
	const blockW = 34
	cursor := "  "
	nameStyle := lipgloss.NewStyle()
	if sel {
		cursor = styleSel.Render("❯ ")
		nameStyle = styleSel
	}
	nameW := lipgloss.Width(name)
	valueW := lipgloss.Width(value)
	gap := blockW - nameW - valueW
	if gap < 2 {
		gap = 2
	}
	return cursor + nameStyle.Render(name) + strings.Repeat(" ", gap) + value
}
