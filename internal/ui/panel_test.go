package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"agent-maid/internal/zcodestore"
)

// TestHelpPanelScrollable 窄矮终端里帮助面板必须能滚到最后一行(内容不能丢),
// 且任意时刻整页都不超出终端。
func TestHelpPanelScrollable(t *testing.T) {
	setupIsolated(t)
	z := NewZcode()
	z.width, z.height = 60, 20
	z = updZ(t, z, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if z.mode != modeHelp {
		t.Fatalf("按 ? 应进入帮助,实际 mode=%v", z.mode)
	}

	content := z.panelContent()
	if !z.panelOverflow(content, zChips) {
		t.Fatal("60x20 下帮助内容应当溢出(否则该用例失去意义)")
	}
	assertScreen(t, "help-top", z.View(), 60, 20)

	// 滚到底,最后一行(存储位置提示与"按任意键返回")必须出现
	for i := 0; i < 60; i++ {
		z = updZ(t, z, tea.KeyMsg{Type: tea.KeyDown})
		assertScreen(t, "help-scrolled", z.View(), 60, 20)
	}
	if z.mode != modeHelp {
		t.Fatal("滚动不应关闭帮助面板")
	}
	view := z.View()
	for _, want := range []string{"槽位存储位置", "按任意键返回"} {
		if !strings.Contains(view, want) {
			t.Errorf("滚到底后仍看不到 %q", want)
		}
	}

	// 任意其他键关闭面板并回到列表
	z = updZ(t, z, tea.KeyMsg{Type: tea.KeyEsc})
	if z.mode != modeNormal {
		t.Fatalf("Esc 应关闭帮助面板,实际 mode=%v", z.mode)
	}

	// 再次打开时滚动位置必须归零
	z = updZ(t, z, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if z.panelTop != 0 {
		t.Fatalf("重开帮助应回到顶部,实际 top=%d", z.panelTop)
	}
}

// TestArmSwitchShowsPrompt 切换需二次确认,第一次 Enter 必须给出可见提示
// (否则用户以为按键没反应,会重复按)。
func TestArmSwitchShowsPrompt(t *testing.T) {
	setupIsolated(t)
	z := NewZcode()
	z.slots = []zcodestore.Slot{{Name: "work"}}
	z.width, z.height = 80, 24

	z = updZ(t, z, tea.KeyMsg{Type: tea.KeyEnter})
	if !z.switchArm {
		t.Fatal("第一次 Enter 应进入待确认态")
	}
	if !strings.Contains(z.View(), "再按一次 Enter 确认切换") {
		t.Fatal("待确认态没有可见提示")
	}

	// 再按 Enter 才真正执行
	z, cmd := updZCmd(t, z, tea.KeyMsg{Type: tea.KeyEnter})
	if z.switchArm {
		t.Fatal("第二次 Enter 应结束待确认态")
	}
	if cmd == nil {
		t.Fatal("第二次 Enter 应触发切换")
	}

	// 待确认态下按其他键应取消
	z.switchArm = false
	z = updZ(t, z, tea.KeyMsg{Type: tea.KeyEnter})
	if !z.switchArm {
		t.Fatal("应重新进入待确认态")
	}
	z = updZ(t, z, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if z.switchArm {
		t.Fatal("按其他键应取消待确认")
	}
}

// updZ 调用 ZcodeModel.Update 并把接口返回值还原成具体类型。
func updZ(t *testing.T, m ZcodeModel, msg tea.Msg) ZcodeModel {
	t.Helper()
	next, _ := updZCmd(t, m, msg)
	return next
}

func updZCmd(t *testing.T, m ZcodeModel, msg tea.Msg) (ZcodeModel, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	typed, ok := next.(ZcodeModel)
	if !ok {
		t.Fatalf("Update 返回了意外类型 %T", next)
	}
	return typed, cmd
}
