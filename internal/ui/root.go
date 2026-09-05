package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"agent-maid/internal/meta"
)

type screen int

const (
	screenWizard   screen = iota // 首次运行设置向导
	screenSelect                 // 目标软件选择
	screenTarget                 // 目标管理器
	screenSettings               // 设置
)

// plainMode 声明在 base.go;向导结果在这里落盘并生效。

// RootModel 顶层模型:设置向导(仅首次)→ 首屏选择目标软件 → 对应管理器。
// 只面向 Manager 接口路由,不感知任何具体工具。
type RootModel struct {
	screen   screen
	registry []Manager // 全部已注册工具
	cur      Manager   // 当前进入的管理器(nil = 首屏)
	pre      string    // CLI 预选的工具 Key(向导完成后直接进入)
	sel      SelectModel
	wiz      WizardModel
	set      SettingsModel
}

// NewRoot 创建根模型。pre 为 CLI 预选的工具 Key(空串则停在首屏)。
func NewRoot(pre string) RootModel {
	registry := newRegistry()
	settings := meta.LoadSettings()
	plainMode = settings.Plain
	noAnim = settings.NoAnim
	m := RootModel{
		screen:   screenSelect,
		registry: registry,
		pre:      pre,
		sel:      NewSelect(registry, settings.LastTarget),
		wiz:      NewWizard(),
		set:      NewSettings(),
	}
	if !settings.SetupDone {
		m.screen = screenWizard
		return m
	}
	for _, mgr := range registry {
		if mgr.Info().Key == pre {
			m.cur = mgr
			m.screen = screenTarget
		}
	}
	return m
}

// rememberTarget 记录上次使用的目标软件,首页预选用。
func rememberTarget(key string) {
	s := meta.LoadSettings()
	if s.LastTarget == key {
		return
	}
	s.LastTarget = key
	_ = meta.SaveSettings(s)
}

func (m RootModel) Init() tea.Cmd {
	switch m.screen {
	case screenWizard:
		return m.wiz.Init()
	case screenSettings:
		return m.set.Init()
	case screenTarget:
		if m.cur != nil {
			return m.cur.Init()
		}
	}
	return m.sel.Init()
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 全局消息:向导完成 / 选择完成 / 返回首屏
	switch msg := msg.(type) {
	case wizardDoneMsg:
		_ = meta.SaveSettings(msg.settings)
		plainMode = msg.settings.Plain
		noAnim = msg.settings.NoAnim
		if m.pre != "" {
			for _, mgr := range m.registry {
				if mgr.Info().Key == m.pre {
					m.cur = mgr
					m.screen = screenTarget
					return m, m.cur.Init()
				}
			}
		}
		m.screen = screenSelect
		return m, m.sel.Init()
	case targetSelectedMsg:
		if msg.Index >= 0 && msg.Index < len(m.registry) {
			m.cur = m.registry[msg.Index]
			m.screen = screenTarget
			rememberTarget(m.cur.Info().Key)
			return m, m.cur.Init()
		}
		return m, nil
	case backMsg:
		m.cur = nil
		m.screen = screenSelect
		return m, m.sel.Init()
	case settingsOpenMsg:
		m.screen = screenSettings
		m.set = NewSettings()
		return m, m.set.Init()
	case reopenWizardMsg:
		m.screen = screenWizard
		m.wiz = NewWizard()
		return m, m.wiz.Init()
	case tea.WindowSizeMsg:
		// 广播给全部注册实例:未进入的管理器也要保持宽度等状态新鲜
		for i, mgr := range m.registry {
			m.registry[i], _ = mgr.Update(msg)
		}
		if m.cur != nil {
			m.cur, _ = m.cur.Update(msg) // 必须接回副本,否则宽度等状态丢失
		}
		var cmd tea.Cmd
		m.wiz, _ = m.wiz.Update(msg)
		m.set, _ = m.set.Update(msg)
		m.sel, cmd = m.sel.Update(msg)
		return m, cmd
	}

	switch m.screen {
	case screenWizard:
		var cmd tea.Cmd
		m.wiz, cmd = m.wiz.Update(msg)
		return m, cmd
	case screenSettings:
		var cmd tea.Cmd
		m.set, cmd = m.set.Update(msg)
		return m, cmd
	case screenSelect:
		var cmd tea.Cmd
		m.sel, cmd = m.sel.Update(msg)
		return m, cmd
	case screenTarget:
		if m.cur != nil {
			var cmd tea.Cmd
			m.cur, cmd = m.cur.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m RootModel) View() string {
	switch m.screen {
	case screenWizard:
		return m.wiz.View()
	case screenSettings:
		return m.set.View()
	case screenTarget:
		if m.cur != nil {
			return m.cur.View()
		}
	}
	return m.sel.View()
}
