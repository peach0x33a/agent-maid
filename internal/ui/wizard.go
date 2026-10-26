// 首次运行设置向导:自检终端的渐变色与特殊符号支持能力。
// 检测结果存入 settings.json;特殊符号不受支持时全局启用纯文本降级。
package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"agent-maid/internal/meta"
)

// wizardDoneMsg 向导完成,携带设置结果。
type wizardDoneMsg struct{ settings meta.Settings }

// WizardModel 设置向导。
type WizardModel struct {
	step      int // 0 渐变色 1 特殊符号
	width     int
	height    int
	gradOK    bool
	symBroken bool
}

// NewWizard 创建向导。
func NewWizard() WizardModel { return WizardModel{} }

// Reset 重置向导进度(保留已知的终端尺寸)。
func (w *WizardModel) Reset() {
	w.step = 0
	w.gradOK = false
	w.symBroken = false
}

func (m WizardModel) Init() tea.Cmd { return nil }

func (m WizardModel) Update(msg tea.Msg) (WizardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			// 跳过向导:保留已有设置,只标记为已完成
			cur := meta.LoadSettings()
			cur.SetupDone = true
			return m, func() tea.Msg { return wizardDoneMsg{settings: cur} }
		}
		if m.step == 0 {
			switch msg.String() {
			case "y":
				m.gradOK = true
				m.step = 1
			case "n":
				m.gradOK = false
				m.step = 1
			}
			return m, nil
		}
		switch msg.String() {
		case "y":
			return m, m.done()
		case "n":
			m.symBroken = true
			return m, m.done()
		}
	}
	return m, nil
}

// done 收尾并落盘设置。
func (m WizardModel) done() tea.Cmd {
	settings := meta.Settings{
		SetupDone: true,
		Truecolor: m.gradOK,
		Symbols:   true,
	}
	if m.symBroken {
		settings.Symbols = false
		settings.Plain = true
	}
	return func() tea.Msg { return wizardDoneMsg{settings: settings} }
}

func (m WizardModel) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	var b strings.Builder
	b.WriteString(styleTitle.Render("Agent Maid |´・ω・)ノ · 首次运行设置向导"))
	b.WriteString("\n\n")

	switch m.step {
	case 0:
		b.WriteString(m.box(fmt.Sprintf("检测 1/2 · %s", styleOk.Render("渐变色"))))
		b.WriteString("\n")
		b.WriteString(gradientBar(w))
		b.WriteString("\n\n")
		b.WriteString("上方色带是否显示为平滑渐变(而不是断层、纯灰或纯黑)?\n\n")
		b.WriteString(styleHelp.Render("y 支持    n 不支持    Esc 跳过向导"))
	case 1:
		b.WriteString(m.box(fmt.Sprintf("检测 2/2 · %s", styleOk.Render("特殊符号"))))
		b.WriteString("\n")
		b.WriteString(stylePlainName.Render("▀▄█  ❯ ●  ⠋⠙⠹⠸  │─┌┐└┘  Agent Maid |´・ω・)ノ"))
		b.WriteString("\n\n")
		b.WriteString("以上字符是否全部正常显示(没有方框、问号或乱码)?\n\n")
		b.WriteString(styleHelp.Render("y 支持    n 不支持(将启用纯文本界面)"))
	}
	return centerScreen(b.String(), w, m.height)
}

// box 向导卡片外框。
func (m WizardModel) box(title string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 2).
		Render(title)
}

// gradientBar 渲染一条平滑的渐变色带供用户目检。
func gradientBar(w int) string {
	n := w - 8
	if n > 60 {
		n = 60
	}
	if n < 10 {
		n = 10
	}
	stops := []string{"#5EEAD4", "#10A37F", "#0EA5E9"}
	var b strings.Builder
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1)
		seg := t * float64(len(stops)-1)
		j := int(seg)
		if j >= len(stops)-1 {
			j = len(stops) - 2
		}
		color := lerpColor(stops[j], stops[j+1], seg-float64(j))
		b.WriteString(lipStyle("", color).Render(" "))
	}
	return b.String()
}
