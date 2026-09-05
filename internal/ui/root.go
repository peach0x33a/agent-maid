package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenSelect screen = iota
	screenTarget
)

// RootModel 顶层模型:首屏选择目标软件,再进入对应管理器。
// 只面向 Manager 接口路由,不感知任何具体工具。
type RootModel struct {
	screen   screen
	registry []Manager // 全部已注册工具
	cur      Manager   // 当前进入的管理器(nil = 首屏)

	sel SelectModel
}

// NewRoot 创建根模型。pre 为 CLI 预选的工具 Key(空串则停在首屏)。
func NewRoot(pre string) RootModel {
	registry := newRegistry()
	m := RootModel{
		screen:   screenSelect,
		registry: registry,
		sel:      NewSelect(registry),
	}
	for _, mgr := range registry {
		if mgr.Info().Key == pre {
			m.cur = mgr
			m.screen = screenTarget
		}
	}
	return m
}

func (m RootModel) Init() tea.Cmd {
	if m.cur != nil {
		return m.cur.Init()
	}
	return m.sel.Init()
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 全局消息:选择完成 / 返回首屏
	switch msg := msg.(type) {
	case targetSelectedMsg:
		if msg.Index >= 0 && msg.Index < len(m.registry) {
			m.cur = m.registry[msg.Index]
			m.screen = screenTarget
			return m, m.cur.Init()
		}
		return m, nil
	case backMsg:
		m.cur = nil
		m.screen = screenSelect
		return m, m.sel.Init()
	case tea.WindowSizeMsg:
		var cmd tea.Cmd
		m.sel, cmd = m.sel.Update(msg)
		if m.cur != nil {
			_, _ = m.cur.Update(msg)
		}
		return m, cmd
	}

	if m.screen == screenSelect {
		var cmd tea.Cmd
		m.sel, cmd = m.sel.Update(msg)
		return m, cmd
	}
	if m.cur != nil {
		var cmd tea.Cmd
		m.cur, cmd = m.cur.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m RootModel) View() string {
	if m.screen == screenTarget && m.cur != nil {
		return m.cur.View()
	}
	return m.sel.View()
}
