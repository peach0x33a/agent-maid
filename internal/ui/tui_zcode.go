// ZCode 账号槽位切换器 TUI(主体复用自 zcode-acc-switch)。
package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"agent-maid/internal/zcode"
	"agent-maid/internal/zcodeapp"
	"agent-maid/internal/zcodestore"
)

// ───────────────────────── 模型 ─────────────────────────

type zconfirm int

const (
	zconfirmApply zconfirm = iota
	zconfirmOverwriteSave
)

// zChips ZCode 界面底部操作栏(与 ChatGPT 界面同一套交互模式)。
var zChips = []chip{
	{"enter", "切换"},
	{"s", "保存"},
	{"S", "快存"},
	{"r", "重命名"},
	{"d", "删除"},
	{"m", "机器信息"},
	{"?", "帮助"},
}

type ZcodeModel struct {
	svc    *zcodeapp.Service
	mode   mode
	quit   bool
	status status

	slots    []zcodestore.Slot
	cursor   int
	current  zcodeapp.CurrentInfo
	zcodeRun bool

	input      textinput.Model
	inputFor   inputPurpose
	renameFrom string

	confirm   zconfirm
	confirmTx string // 确认附带的上下文(槽位名)

	chipIdx    int    // 操作栏选中项
	deleteArm  bool   // 已按过一次 d,等待再次确认
	deleteName string // 待删除的槽位名

	waitingFor string
	waitProcs  []zcode.Proc
	waitStart  time.Time
}

func NewZcode() ZcodeModel {
	ti := textinput.New()
	ti.Placeholder = "槽位名,如 work"
	ti.CharLimit = 64
	return ZcodeModel{svc: zcodeapp.New(), input: ti}
}

func (m ZcodeModel) Init() tea.Cmd { return m.refresh }

type zLoadedMsg struct {
	slots    []zcodestore.Slot
	current  zcodeapp.CurrentInfo
	zcodeRun bool
}

// waitTickMsg 等待 ZCode 退出的轮询节拍。
type waitTickMsg struct{}

// zApplyResultMsg 异步执行切换的结果。
type zApplyResultMsg struct {
	name   string
	backup string
	err    error
	waited bool // 是否经历了"等待 ZCode 退出"
	forced bool // 是否为无视风险的立即替换
}

func waitTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return waitTickMsg{} })
}

func (m ZcodeModel) applyCmd(name string, waited, forced bool) tea.Cmd {
	return func() tea.Msg {
		backup, err := m.svc.Apply(name)
		return zApplyResultMsg{name: name, backup: backup, err: err, waited: waited, forced: forced}
	}
}

func (m ZcodeModel) refresh() tea.Msg {
	slots, _ := m.svc.Store.List()
	current := m.svc.Current()
	return zLoadedMsg{slots: slots, current: current, zcodeRun: zcode.ZCodeRunning()}
}

func (m *ZcodeModel) applyLoaded(lm zLoadedMsg) {
	m.slots = lm.slots
	m.current = lm.current
	m.zcodeRun = lm.zcodeRun
	m.clampCursor()
}

func (m *ZcodeModel) clampCursor() {
	if m.cursor >= len(m.slots) {
		m.cursor = len(m.slots) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m ZcodeModel) selected() *zcodestore.Slot {
	if m.cursor >= 0 && m.cursor < len(m.slots) {
		return &m.slots[m.cursor]
	}
	return nil
}

// ───────────────────────── 更新 ─────────────────────────

type zRefreshMsg struct{}

func (m ZcodeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, nil
	case zRefreshMsg:
		return m, m.refresh
	case zLoadedMsg:
		m.applyLoaded(msg)
		return m, nil
	case zApplyResultMsg:
		return m.handleApplyResult(msg)
	}

	switch m.mode {
	case modeInput:
		return m.updateInput(msg)
	case modeConfirm:
		return m.updateConfirm(msg)
	case modeWaiting:
		return m.updateWaiting(msg)
	case modeHelp, modeMachine:
		if _, ok := msg.(tea.KeyMsg); ok {
			m.mode = modeNormal
		}
		return m, nil
	}
	return m.updateNormal(msg)
}

// handleApplyResult 统一呈现切换结果。
func (m ZcodeModel) handleApplyResult(msg zApplyResultMsg) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	switch {
	case msg.err != nil:
		m.status = status{2, msg.err.Error()}
	case msg.forced:
		text := fmt.Sprintf("已无视风险立即切换到槽位 %q", msg.name)
		if msg.backup != "" {
			text += "(原账号已自动备份到 " + msg.backup + ")"
		}
		text += ";ZCode 仍在运行,可能在退出前覆盖回写,建议尽快重启 ZCode"
		m.status = status{3, text}
	case msg.waited:
		text := "ZCode 已完全退出,已自动切换到槽位 " + msg.name
		if msg.backup != "" {
			text += "(原账号已自动备份到 " + msg.backup + ")"
		}
		m.status = status{1, text}
	default:
		text := "已切换到槽位 " + msg.name
		if msg.backup != "" {
			text += "(原账号已自动备份到 " + msg.backup + ")"
		}
		m.status = status{1, text}
	}
	return m, func() tea.Msg { return zRefreshMsg{} }
}

func (m ZcodeModel) updateNormal(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "q", "Q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "esc":
		return m, backCmd
	case "left", "right":
		if key.String() == "left" {
			m.chipIdx = (m.chipIdx + len(zChips) - 1) % len(zChips)
		} else {
			m.chipIdx = (m.chipIdx + 1) % len(zChips)
		}
		return m, nil
	case "up", "k":
		m.deleteArm = false
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		m.deleteArm = false
		if m.cursor < len(m.slots)-1 {
			m.cursor++
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.slots) - 1
	case "enter":
		// 回车执行操作栏当前选中项;默认第 0 项即"切换"
		return m.dispatch(zChips[m.chipIdx].key)
	default:
		return m.dispatch(key.String())
	}
	return m, nil
}

// dispatch 统一的动作分发:字母键与操作栏回车共用。
func (m ZcodeModel) dispatch(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "s": // 保存当前账号
		if !m.current.Exists {
			m.status = status{2, "没有找到 credentials.json,无账号可保存"}
			return m, nil
		}
		m.mode = modeInput
		m.inputFor = inputSave
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	case "S": // 快速保存
		if !m.current.Exists {
			m.status = status{2, "没有找到 credentials.json,无账号可保存"}
			return m, nil
		}
		name := time.Now().Format("snapshot-0102-150405")
		slot, err := m.svc.SaveCurrent(name, false)
		m.setStatus(err, "已保存到槽位 "+describeSlot(slot), "保存失败")
		return m, func() tea.Msg { return zRefreshMsg{} }
	case "r": // 重命名
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		m.mode = modeInput
		m.inputFor = inputRename
		m.renameFrom = sel.Name
		m.input.SetValue(sel.Name)
		m.input.Focus()
		return m, textinput.Blink
	case "d": // 删除:再按一次 d 确认
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		if m.deleteArm && m.deleteName == sel.Name {
			m.deleteArm = false
			err := m.svc.Store.Delete(sel.Name)
			m.setStatus(err, "已删除槽位 "+sel.Name, "删除失败")
			return m, func() tea.Msg { return zRefreshMsg{} }
		}
		m.deleteArm = true
		m.deleteName = sel.Name
		m.status = status{4, fmt.Sprintf("再按一次 d 删除槽位 %q", sel.Name)}
		return m, nil
	case "m":
		m.mode = modeMachine
	case "?", "h":
		m.mode = modeHelp
	case "enter": // 应用选中槽位
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		m.confirm = zconfirmApply
		m.confirmTx = sel.Name
		m.mode = modeConfirm
	default:
		m.deleteArm = false
	}
	return m, nil
}

func (m ZcodeModel) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if ok {
		switch key.String() {
		case "esc":
			m.mode = modeNormal
			m.input.Blur()
			return m, nil
		case "enter":
			name := strings.TrimSpace(m.input.Value())
			var err error
			var extra string
			switch m.inputFor {
			case inputSave:
				if m.StoreHas(name) {
					// 已存在:转入覆盖确认,避免静默覆盖
					m.confirm = zconfirmOverwriteSave
					m.confirmTx = name
					m.mode = modeConfirm
					m.input.Blur()
					return m, nil
				}
				var slot zcodestore.Slot
				slot, err = m.svc.SaveCurrent(name, false)
				if err == nil {
					extra = "已保存到槽位 " + describeSlot(slot)
				}
			case inputRename:
				err = m.svc.Store.Rename(m.renameFrom, name)
			}
			if err == nil {
				m.status = status{1, orDefault(extra, "操作成功")}
				m.mode = modeNormal
				m.input.Blur()
				return m, func() tea.Msg { return zRefreshMsg{} }
			}
			m.status = status{2, err.Error()}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m ZcodeModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "n", "esc", "q", "Q":
		m.mode = modeNormal
		return m, nil
	case "y", "enter":
		switch m.confirm {
		case zconfirmApply:
			if m.zcodeRun {
				// 默认路径:等待 ZCode 完全退出后再执行替换
				m.waitingFor = m.confirmTx
				m.waitProcs = zcode.ZCodeProcs()
				m.waitStart = time.Now()
				m.mode = modeWaiting
				return m, waitTick()
			}
			return m, m.applyCmd(m.confirmTx, false, false)
		case zconfirmOverwriteSave:
			slot, err := m.svc.SaveCurrent(m.confirmTx, true)
			m.setStatus(err, "已覆盖保存到槽位 "+describeSlot(slot), "保存失败")
			m.mode = modeNormal
			return m, func() tea.Msg { return zRefreshMsg{} }
		}
	case "f":
		// 无视风险:立即替换,不等 ZCode 退出
		if m.confirm == zconfirmApply && m.zcodeRun {
			return m, m.applyCmd(m.confirmTx, false, true)
		}
	}
	return m, nil
}

func (m ZcodeModel) updateWaiting(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(waitTickMsg); ok {
		procs := zcode.ZCodeProcs()
		if len(procs) == 0 {
			// ZCode 已完全退出,安全执行替换
			return m, m.applyCmd(m.waitingFor, true, false)
		}
		m.waitProcs = procs
		return m, waitTick()
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "f":
		return m, m.applyCmd(m.waitingFor, false, true)
	case "n", "esc":
		m.mode = modeNormal
		m.status = status{0, "已取消等待,凭据未做任何更改"}
		return m, nil
	case "q", "Q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	}
	return m, nil
}

func (m ZcodeModel) StoreHas(name string) bool { return m.svc.Store.Has(name) }

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func describeSlot(slot zcodestore.Slot) string {
	return fmt.Sprintf("%q(%s)", slot.Name, zcodeAccountLabel(slot.Meta))
}

// pidList 返回当前 ZCode 进程 PID 的紧凑列表,用于确认框提示。
func (m ZcodeModel) pidList() string {
	procs := zcode.ZCodeProcs()
	pids := make([]string, 0, len(procs))
	for _, p := range procs {
		pids = append(pids, strconv.Itoa(p.Pid))
	}
	if len(pids) == 0 {
		return "检测中"
	}
	if len(pids) > 4 {
		pids = append(pids[:4], "…")
	}
	return strings.Join(pids, ",")
}

func (m *ZcodeModel) setStatus(err error, okText, errPrefix string) {
	if err != nil {
		m.status = status{2, errPrefix + ": " + err.Error()}
	} else {
		m.status = status{1, okText}
	}
}

// ───────────────────────── 视图 ─────────────────────────

func (m ZcodeModel) currentLine() string {
	if !m.current.Exists {
		return styleWarn.Render("当前账号:未找到 credentials.json")
	}
	line := "当前账号:" + zcodeAccountLabel(m.current.Meta)
	if m.current.Exists && !m.current.Decrypt {
		line += styleWarn.Render("(无法解密:密钥不匹配,可能设置了 ZCODE_CREDENTIAL_SECRET)")
	}
	return line
}

func (m ZcodeModel) slotRow(i int) string {
	slot := m.slots[i]
	cursor := "  "
	nameStyle := lipgloss.NewStyle()
	if i == m.cursor {
		cursor = styleSel.Render("❯ ")
		nameStyle = styleSel
	}
	mark := ""
	if m.current.Exists && zcode.EqualCredentials(m.current.Creds, slot.Credentials) {
		mark = styleOk.Render(" ●使用中")
	}
	warn := ""
	if slot.KeyFingerprint != zcode.KeyFingerprint(zcode.DeriveKey(zcode.CredentialSecret())) {
		warn = styleWarn.Render(" ⚠跨机/跨密钥")
	}
	meta := zcodeAccountLabel(slot.Meta)
	name := slot.Name
	if len(name) > 24 {
		name = name[:24]
	}
	return fmt.Sprintf("%s%s %s%s%s", cursor, nameStyle.Render(fmt.Sprintf("%-26s", name)), styleDim.Render(meta), mark, warn)
}

func (m ZcodeModel) View() string {
	if m.quit {
		return ""
	}
	var b strings.Builder
	b.WriteString(banner(TargetZCode))
	b.WriteString("\n")
	b.WriteString(m.currentLine())
	b.WriteString("\n")
	if m.zcodeRun {
		b.WriteString(styleWarn.Render("⚠ ZCode 正在运行:切换将默认等待其完全退出后执行,确认时也可选择无视风险立即替换"))
		b.WriteString("\n")
	}
	b.WriteString(styleHeader.Render(fmt.Sprintf(" 槽位(共 %d 个)", len(m.slots))))
	b.WriteString("\n")

	if len(m.slots) == 0 {
		b.WriteString(styleDim.Render("  (空)按 s 保存当前账号,或 S 快速保存\n"))
	} else {
		for i := range m.slots {
			b.WriteString(m.slotRow(i))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.status.render())
	b.WriteString("\n")

	switch m.mode {
	case modeInput:
		purpose := "输入槽位名保存当前账号"
		if m.inputFor == inputRename {
			purpose = "重命名槽位 " + m.renameFrom
		}
		b.WriteString("\n" + styleBox.Render(purpose+"\n"+m.input.View()) + "\n")
	case modeConfirm:
		var text string
		var keys string
		switch m.confirm {
		case zconfirmApply:
			if m.zcodeRun {
				text = fmt.Sprintf("将切换到槽位 %q,并写入 credentials.json。\n⚠ ZCode 正在运行(PID %s):运行中替换可能被其退出前的凭据回写覆盖。",
					m.confirmTx, m.pidList())
				keys = "[y] 等待 ZCode 完全退出后自动切换(推荐)  [f] 立即替换(无视风险)  [n] 取消"
			} else {
				text = fmt.Sprintf("将切换到槽位 %q,并写入 credentials.json。继续?", m.confirmTx)
				keys = "[y] 确认  [n] 取消"
			}
		case zconfirmOverwriteSave:
			text = fmt.Sprintf("槽位 %q 已存在,覆盖?", m.confirmTx)
			keys = "[y] 确认  [n] 取消"
		}
		b.WriteString("\n" + styleBox.Render(styleWarn.Render(text)+"\n"+keys) + "\n")
	case modeWaiting:
		elapsed := time.Since(m.waitStart).Truncate(time.Second)
		lines := []string{
			styleTitle.Render(fmt.Sprintf("⏳ 等待 ZCode 完全退出…(已等待 %v)", elapsed)),
			fmt.Sprintf("ZCode 退出后将自动切换到槽位 %q 并写入 credentials.json。", m.waitingFor),
			"",
			styleDim.Render("运行中的 ZCode 进程:"),
		}
		for _, p := range m.waitProcs {
			cmd := p.Cmd
			if len(cmd) > 90 {
				cmd = cmd[:90] + "…"
			}
			lines = append(lines, fmt.Sprintf("  PID %d  %s", p.Pid, cmd))
		}
		if len(m.waitProcs) == 0 {
			lines = append(lines, styleDim.Render("  (检测中…)"))
		}
		lines = append(lines, "", styleWarn.Render("[f] 立即替换(无视风险)")+"  [n] 取消等待  [q] 退出")
		b.WriteString("\n" + styleBox.Render(strings.Join(lines, "\n")) + "\n")
	case modeHelp:
		b.WriteString("\n" + styleBox.Render(zHelpView()) + "\n")
	case modeMachine:
		info := zcode.GetMachineInfo()
		val := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		accentSt := lipgloss.NewStyle().Foreground(targets[1].accent)
		lines := []string{
			styleTitle.Render("机器信息"),
			styleDim.Render("平台:      ") + val.Render(info.Platform),
			styleDim.Render("Home:      ") + val.Render(info.HomeDir),
			styleDim.Render("用户:      ") + val.Render(info.Username),
			styleDim.Render("密钥来源:  ") + val.Render(info.SecretSrc),
			styleDim.Render("密钥指纹:  ") + accentSt.Render(info.Fingerprint),
			styleDim.Render("凭据目录:  ") + val.Render(zcode.V2Dir()),
			styleDim.Render("deviceMid: ") + val.Render(orDefault(info.DeviceMid, "(未生成)")),
			"",
			styleDim.Render("按任意键返回"),
		}
		b.WriteString("\n" + styleBox.Render(strings.Join(lines, "\n")) + "\n")
	}

	// 底部操作栏:左右键选中、回车执行,或直接按对应键
	b.WriteString("\n" + chipBar(zChips, m.chipIdx, targets[1].accent))
	b.WriteString("\n" + styleDim.Render("↑/↓ 选条目 · ←/→ 选操作 · 回车执行选中操作 · Esc 返回选择 · q/Q 退出"))
	return b.String()
}

func zHelpView() string {
	return strings.Join([]string{
		styleTitle.Render("按键说明(ZCode 账号切换器)"),
		"j/k 或 ↑/↓   移动光标",
		"←/→          在底部操作栏中选择操作,回车执行",
		"enter        应用选中槽位(切换账号,自动备份未覆盖的当前账号)",
		styleDim.Render("             ZCode 运行中时,默认等待其完全退出后自动切换;"),
		styleDim.Render("             确认/等待界面按 f 可无视风险立即替换。"),
		"s            保存当前账号到新槽位(输入名字)",
		"S            快速保存当前账号(自动命名)",
		"r            重命名选中槽位",
		"d            删除选中槽位(按两次 d 确认)",
		"m            查看机器信息(密钥指纹/deviceMid)",
		"?            本帮助",
		"Esc          返回目标软件选择",
		"q / Q / Ctrl+C 退出",
		"",
		styleDim.Render("槽位存储位置:" + zcodestore.DefaultDir()),
		"",
		styleDim.Render("按任意键返回"),
	}, "\n")
}
