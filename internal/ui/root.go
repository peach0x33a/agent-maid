package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenSelect screen = iota
	screenTarget
)

// RootModel 顶层模型:首屏选择目标软件,再进入对应切换器。
type RootModel struct {
	screen screen
	target Target

	sel   SelectModel
	zcode ZcodeModel
	gpt   ChatgptModel
}

// NewRoot 创建根模型。pre 非 TargetNone 时跳过首屏直接进入目标。
func NewRoot(pre Target) RootModel {
	m := RootModel{screen: screenSelect, sel: NewSelect(), zcode: NewZcode(), gpt: NewChatgpt()}
	if pre != TargetNone {
		m.screen = screenTarget
		m.target = pre
	}
	return m
}

func (m RootModel) Init() tea.Cmd {
	if m.screen == screenTarget {
		return m.activeInit()
	}
	return m.sel.Init()
}

func (m RootModel) activeInit() tea.Cmd {
	switch m.target {
	case TargetZCode:
		return m.zcode.Init()
	case TargetChatGPT:
		return m.gpt.Init()
	}
	return nil
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 全局消息:选择完成 / 返回首屏
	switch msg := msg.(type) {
	case targetSelectedMsg:
		m.screen = screenTarget
		m.target = msg.target
		return m, m.activeInit()
	case backMsg:
		m.screen = screenSelect
		return m, m.sel.Init()
	case tea.WindowSizeMsg:
		var cmd tea.Cmd
		m.sel, cmd = forwardSelect(m.sel, msg)
		m.zcode, _ = forwardZcode(m.zcode, msg)
		m.gpt, _ = forwardGpt(m.gpt, msg)
		return m, cmd
	}

	switch m.screen {
	case screenSelect:
		var cmd tea.Cmd
		m.sel, cmd = forwardSelect(m.sel, msg)
		return m, cmd
	case screenTarget:
		switch m.target {
		case TargetZCode:
			var cmd tea.Cmd
			m.zcode, cmd = forwardZcode(m.zcode, msg)
			return m, cmd
		case TargetChatGPT:
			var cmd tea.Cmd
			m.gpt, cmd = forwardGpt(m.gpt, msg)
			return m, cmd
		}
	}
	return m, nil
}

func forwardSelect(m SelectModel, msg tea.Msg) (SelectModel, tea.Cmd) {
	return m.Update(msg)
}

func forwardZcode(m ZcodeModel, msg tea.Msg) (ZcodeModel, tea.Cmd) {
	nm, cmd := m.Update(msg)
	return nm.(ZcodeModel), cmd
}

func forwardGpt(m ChatgptModel, msg tea.Msg) (ChatgptModel, tea.Cmd) {
	nm, cmd := m.Update(msg)
	return nm.(ChatgptModel), cmd
}

func (m RootModel) View() string {
	switch m.screen {
	case screenTarget:
		switch m.target {
		case TargetZCode:
			return m.zcode.View()
		case TargetChatGPT:
			return m.gpt.View()
		}
	}
	return m.sel.View()
}
