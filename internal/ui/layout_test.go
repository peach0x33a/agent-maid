package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"agent-maid/internal/antigravity"
	"agent-maid/internal/gptstore"
	"agent-maid/internal/meta"
	"agent-maid/internal/zcodestore"
)

// setupIsolated 把配置目录指到临时目录,避免测试读写真实账号库。
func setupIsolated(t *testing.T) {
	t.Helper()
	t.Setenv("AGENT_MAID_DIR", t.TempDir())
	t.Setenv("ZCODE_ACC_SWITCH_DIR", t.TempDir())
	t.Setenv("ANTIGRAVITY_STORE_DIR", t.TempDir())
	t.Setenv("ZCODE_V2_DIR", t.TempDir())
	t.Setenv("ANTIGRAVITY_GEMINI_DIR", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	plainMode, noAnim = false, true
}

// assertScreen 断言整页不超过终端尺寸(超出会让终端整体滚动,
// 标题与底栏被推出可视区)。
func assertScreen(t *testing.T, name string, view string, w, h int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > h {
		t.Errorf("%s: 高度 %d 行超过终端 %d 行", name, len(lines), h)
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got > w {
			t.Errorf("%s: 第 %d 行宽 %d 超过终端 %d", name, i+1, got, w)
		}
	}
}

// TestScreensFitTerminal 覆盖三个管理器的常规页与浮层页,在常见终端尺寸下
// 都不能超出可视区。
func TestScreensFitTerminal(t *testing.T) {
	setupIsolated(t)

	// 造 30 个槽位/账号,覆盖"列表本身超屏"的场景
	zstore := &zcodestore.Store{Dir: t.TempDir()}
	for i := 0; i < 30; i++ {
		name := strings.Repeat("s", 3) + string(rune('a'+i%26))
		if _, err := zstore.Save(name, zcodestore.Slot{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	gst, _ := gptstore.Load(gptstore.DefaultPath())
	for i := 0; i < 30; i++ {
		gst.Upsert(&gptstore.Account{ID: "id-" + string(rune('a'+i%26)), Label: "user@example.com"})
	}
	if err := gst.Save(); err != nil {
		t.Fatal(err)
	}

	sizes := [][2]int{{80, 24}, {100, 30}, {60, 20}, {140, 40}}
	modes := []struct {
		name string
		set  func(baseModel) baseModel
	}{
		{"normal", func(b baseModel) baseModel { return b }},
		{"help", func(b baseModel) baseModel { b.mode = modeHelp; return b }},
		{"machine", func(b baseModel) baseModel { b.mode = modeMachine; return b }},
		{"input", func(b baseModel) baseModel { b.mode = modeInput; return b }},
		{"confirm", func(b baseModel) baseModel { b.mode = modeConfirm; return b }},
	}

	for _, size := range sizes {
		w, h := size[0], size[1]
		for _, md := range modes {
			c := NewChatgpt()
			c.baseModel = md.set(c.baseModel)
			c.width, c.height = w, h
			assertScreen(t, "chatgpt/"+md.name, c.View(), w, h)

			z := NewZcode()
			z.baseModel = md.set(z.baseModel)
			z.width, z.height = w, h
			assertScreen(t, "zcode/"+md.name, z.View(), w, h)

			a := NewAntigravity()
			a.baseModel = md.set(a.baseModel)
			a.width, a.height = w, h
			assertScreen(t, "antigravity/"+md.name, a.View(), w, h)
		}
	}
}

// TestStartScreenFitsTerminal 首屏、向导、设置页同样不能超屏。
func TestStartScreenFitsTerminal(t *testing.T) {
	setupIsolated(t)
	for _, size := range [][2]int{{80, 24}, {60, 20}, {140, 40}} {
		w, h := size[0], size[1]

		sel := NewSelect([]Manager{NewChatgpt(), NewZcode(), NewAntigravity()}, "")
		sel, _ = sel.Update(tea.WindowSizeMsg{Width: w, Height: h})
		assertScreen(t, "select", sel.View(), w, h)

		wiz := NewWizard()
		wiz, _ = wiz.Update(tea.WindowSizeMsg{Width: w, Height: h})
		assertScreen(t, "wizard", wiz.View(), w, h)

		set := NewSettings()
		set, _ = set.Update(tea.WindowSizeMsg{Width: w, Height: h})
		assertScreen(t, "settings", set.View(), w, h)
	}
}

// TestChipBarWraps 操作栏在窄终端必须折行而不是被裁剪掉后半截。
func TestChipBarWraps(t *testing.T) {
	chips := []chip{
		{"enter", "[↵] 切换"}, {"s", "[S] 保存"}, {"i", "[I] 导入"},
		{"r", "[R] 重命名"}, {"d", "[D] 删除"}, {"z", "[Z] 撤销"},
		{"n", "[N] 登出"}, {"m", "[M] 信息"}, {"?", "[?] 帮助"},
	}
	for _, w := range []int{40, 60, 80, 100, 160} {
		lines := chipBarLines(chips, 0, lipgloss.Color("212"), w)
		for i, l := range lines {
			if got := lipgloss.Width(l); got > w {
				t.Errorf("宽 %d 时第 %d 行宽 %d", w, i+1, got)
			}
		}
		joined := strings.Join(lines, "")
		for _, c := range chips {
			if !strings.Contains(joined, strings.TrimSpace(c.disp)) {
				t.Errorf("宽 %d 时操作 %q 被丢弃", w, c.disp)
			}
		}
	}
}

// TestSettingsCursorWrap 设置页光标必须始终可见(上下循环 3 项)。
func TestSettingsCursorWrap(t *testing.T) {
	setupIsolated(t)
	m := NewSettings()
	seen := map[int]bool{}
	for i := 0; i < 10; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
		if m.cursor < 0 || m.cursor >= settingsItems {
			t.Fatalf("上键后 cursor=%d 越界", m.cursor)
		}
		seen[m.cursor] = true
		if !strings.Contains(m.View(), "❯") {
			t.Fatalf("cursor=%d 时没有可见光标", m.cursor)
		}
	}
	if !seen[settingsItems-1] {
		t.Fatal("上键循环不到最后一项")
	}
}

// TestRenameSameNameKeepsSlot 同名重命名不能把槽位删掉。
func TestRenameSameNameKeepsSlot(t *testing.T) {
	setupIsolated(t)
	dir := meta.ZcodeBaseDir()
	s := &zcodestore.Store{Dir: dir}
	if _, err := s.Save("work", zcodestore.Slot{Name: "work"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename("work", "work"); err != nil {
		t.Fatalf("同名重命名报错: %v", err)
	}
	if _, err := s.Get("work"); err != nil {
		t.Fatal("同名重命名后槽位消失了")
	}
}

// TestRenameOntoExistingRejected 重命名撞名必须报错,不能静默覆盖目标槽位。
func TestRenameOntoExistingRejected(t *testing.T) {
	setupIsolated(t)
	s := &zcodestore.Store{Dir: meta.ZcodeBaseDir()}
	if _, err := s.Save("a", zcodestore.Slot{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save("b", zcodestore.Slot{Name: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename("a", "b"); err == nil {
		t.Fatal("重命名到已存在的槽位应当报错")
	}
	if _, err := s.Get("b"); err != nil {
		t.Fatal("目标槽位不应被破坏")
	}
}

// TestAntigravityRenameGuards Antigravity 的槽位重命名同样不能丢/覆盖数据。
func TestAntigravityRenameGuards(t *testing.T) {
	setupIsolated(t)
	s := antigravity.NewStore()
	s.Dir = t.TempDir()
	if _, err := s.Save("work", antigravity.Slot{Name: "work", Token: antigravity.Token(`{"token":{"access_token":"a"}}`)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename("work", "work"); err != nil {
		t.Fatalf("同名重命名报错: %v", err)
	}
	if _, err := s.Get("work"); err != nil {
		t.Fatal("同名重命名后槽位消失了")
	}
	if _, err := s.Save("other", antigravity.Slot{Name: "other"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename("work", "other"); err == nil {
		t.Fatal("重命名到已存在的槽位应当报错")
	}
}
