// agy(Antigravity CLI)账号槽位切换器。
// 实现 Manager 接口接入 agent-maid;共享交互由 baseModel 提供,
// 本文件只包含 agy 自身的数据与动作。
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"agent-maid/internal/antigravity"
)

// ainfo agy 的展示信息。
var ainfo = targetInfo{
	Key:      "antigravity",
	Name:     "Antigravity",
	Desc:     "agy CLI 账号切换",
	Path:     "~/.gemini/antigravity-cli/antigravity-oauth-token",
	pxLarge:  antigravityPxLarge,
	pxSmall:  antigravityPxSmall,
	colLarge: antigravityColLarge,
	colSmall: antigravityColSmall,
	grad:     []string{"#86C561", "#FD553F", "#3087FE"}, // 官方渐变:绿→红→蓝(仅置灰外场景备用)
	accent:   lipgloss.Color("#3087FE"),
}

// aChips agy 界面底部操作栏(与其他工具同一套交互模式)。
var aChips = []chip{
	{"enter", "[↵] 切换"},
	{"s", "[S] 保存"},
	{"r", "[R] 重命名"},
	{"d", "[D] 删除"},
	{"z", "[Z] 撤销"},
	{"n", "[N] 登出"},
	{"m", "[M] 信息"},
	{"?", "[?] 帮助"},
}

type aconfirm int

const (
	aconfirmOverwriteSave aconfirm = iota
)

type AntigravityModel struct {
	baseModel

	svc        *antigravity.Service
	positioned bool     // 首次加载数据后已把光标定位到使用中的账号
	trashStack []string // 回收站文件名栈(撤销用)
	slots      []antigravity.Slot
	cursor     int
	current    antigravity.CurrentInfo
	agRun      bool

	input      textinput.Model
	inputFor   inputPurpose
	renameFrom string

	confirm   aconfirm
	confirmTx string
}

// NewAntigravity 创建 agy 管理器。
func NewAntigravity() AntigravityModel {
	svc := antigravity.New()
	_ = svc.Store.PurgeTrash() // 上次会话的回收站:正式删除(每次启动一次)
	ti := textinput.New()
	ti.Placeholder = "槽位名,如 work"
	ti.CharLimit = 64
	return AntigravityModel{svc: svc, input: ti}
}

// Info 实现 Manager。
func (m AntigravityModel) Info() *targetInfo { return &ainfo }

// Chips 实现 Manager。
func (m AntigravityModel) Chips() []chip { return aChips }

func (m AntigravityModel) Init() tea.Cmd { return m.refresh }

// ───────────────────────── 数据 ─────────────────────────

type aLoadedMsg struct {
	slots   []antigravity.Slot
	current antigravity.CurrentInfo
	agRun   bool
}

type aApplyResultMsg struct {
	name   string
	backup string
	err    error
}

type aRefreshMsg struct{}

func (m AntigravityModel) refresh() tea.Msg {
	slots, _ := m.svc.Store.List()
	return aLoadedMsg{slots: slots, current: m.svc.Current(), agRun: antigravity.Running()}
}

func (m *AntigravityModel) applyLoaded(lm aLoadedMsg) {
	m.slots = lm.slots
	m.current = lm.current
	m.agRun = lm.agRun
	// 首次加载数据时,把光标定位到使用中的账号
	if !m.positioned {
		m.positioned = true
		if m.current.Exists {
			for i := range m.slots {
				if antigravity.EqualToken(m.current.Token, m.slots[i].Token) {
					m.cursor = i
					break
				}
			}
		}
	}
	m.clampCursor()
}

func (m *AntigravityModel) clampCursor() {
	if m.cursor >= len(m.slots) {
		m.cursor = len(m.slots) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m AntigravityModel) selected() *antigravity.Slot {
	if m.cursor >= 0 && m.cursor < len(m.slots) {
		return &m.slots[m.cursor]
	}
	return nil
}

// defaultSlotName 用当前账号邮箱生成默认槽位名(取 @ 前缀)。
func (m AntigravityModel) defaultSlotName() string {
	email := m.current.Email
	if i := strings.Index(email, "@"); i > 0 {
		email = email[:i]
	}
	if email == "" {
		return time.Now().Format("snapshot-0102-150405")
	}
	return email
}

// ───────────────────────── 更新 ─────────────────────────

func (m AntigravityModel) Update(msg tea.Msg) (Manager, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case aRefreshMsg:
		return m, m.refresh
	case aLoadedMsg:
		m.applyLoaded(msg)
		return m, nil
	case aApplyResultMsg:
		return m.handleApplyResult(msg)
	case statusTickMsg:
		cmd := m.handleStatusTick()
		return m, cmd
	}

	switch m.mode {
	case modeInput:
		return m.updateInput(msg)
	case modeConfirm:
		return m.updateConfirm(msg)
	case modeHelp, modeMachine:
		if key, ok := msg.(tea.KeyMsg); ok {
			if m.scrollPanel(key.String(), m.panelContent(), aChips) {
				return m, nil
			}
			m.mode = modeNormal
		}
		return m, nil
	}
	return m.updateNormal(msg)
}

func (m AntigravityModel) handleApplyResult(msg aApplyResultMsg) (Manager, tea.Cmd) {
	m.mode = modeNormal
	switch {
	case msg.err != nil:
		m.status = status{statusErr, msg.err.Error()}
	default:
		m.status = status{statusOK, describeApply(msg.name, msg.backup)}
	}
	return m, func() tea.Msg { return aRefreshMsg{} }
}

// reload 同步重新读盘(当前账号/槽位/运行状态)。
func (m *AntigravityModel) reload() {
	if lm, ok := m.refresh().(aLoadedMsg); ok {
		m.applyLoaded(lm)
	}
}

func (m AntigravityModel) updateNormal(msg tea.Msg) (Manager, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		m.quit = true
		_ = m.svc.Store.PurgeTrash()
		return m, tea.Quit
	case "q", "Q":
		cmd := m.handleQuitKey()
		if m.quit {
			_ = m.svc.Store.PurgeTrash()
		}
		return m, cmd
	case "esc":
		return m, backCmd
	case "ctrl+r":
		m.clearPrompt()
		m.reload()
		m.status = status{statusOK, "已重新读取当前账号与槽位"}
		return m, nil
	case "left":
		m.chipNav(-1, len(aChips))
		return m, nil
	case "right":
		m.chipNav(1, len(aChips))
		return m, nil
	case "up", "k":
		m.clearPrompt()
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		m.clearPrompt()
		if m.cursor < len(m.slots)-1 {
			m.cursor++
		}
	case "g", "home":
		m.clearPrompt()
		m.cursor = 0
	case "G", "end":
		m.clearPrompt()
		m.cursor = len(m.slots) - 1
	case "enter":
		return m.dispatch(aChips[m.chipIdx].key)
	default:
		return m.dispatch(key.String())
	}
	return m, nil
}

// dispatch 统一的动作分发:字母键与操作栏回车共用。
func (m AntigravityModel) dispatch(key string) (Manager, tea.Cmd) {
	switch key {
	case "s", "S": // 保存当前账号
		// agy 可能在 TUI 开着时刚登录/刚退出登录:先同步读盘再判断
		m.reload()
		if !m.current.Exists {
			m.status = status{statusErr, "没有找到 agy 登录凭据,请先在 agy 里登录"}
			return m, nil
		}
		m.clearPrompt()
		m.mode = modeInput
		m.inputFor = inputSave
		m.input.SetValue(m.defaultSlotName())
		m.input.CursorEnd()
		m.input.Focus()
		return m, textinput.Blink
	case "r", "R": // 重命名
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		m.clearPrompt()
		m.mode = modeInput
		m.inputFor = inputRename
		m.renameFrom = sel.Name
		m.input.SetValue(sel.Name)
		m.input.Focus()
		return m, textinput.Blink
	case "d", "D": // 删除:再按一次 d 确认,5s 不动自动撤下
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		if m.deleteArm && m.deleteName == sel.Name {
			m.clearPrompt()
			trashName, err := m.svc.Store.DeleteToTrash(sel.Name)
			if err == nil {
				m.trashStack = append(m.trashStack, trashName)
				m.setStatus(nil, "已删除槽位 "+sel.Name+"(移入回收站,退出前可按 z 撤销)", "")
			} else {
				m.setStatus(err, "", "删除失败")
			}
			return m, func() tea.Msg { return aRefreshMsg{} }
		}
		cmd := m.armPrompt(sel.Name, fmt.Sprintf("再按一次 d 删除槽位 %q", sel.Name))
		return m, cmd
	case "z", "Z": // 撤销:从回收站恢复最近删除的槽位
		name, err := m.svc.Store.RestoreLatestFromTrash()
		if err != nil {
			m.status = status{statusWarn, "没有可撤销的删除"}
			return m, nil
		}
		if n := len(m.trashStack); n > 0 {
			m.trashStack = m.trashStack[:n-1]
		}
		m.status = status{statusOK, "已恢复槽位 " + name}
		return m, func() tea.Msg { return aRefreshMsg{} }
	case "n", "N": // 登出:清空 agy 登录凭据(自动备份),用于换账号重新登录
		if !m.current.Exists {
			m.status = status{statusErr, "当前没有登录凭据,无需登出"}
			return m, nil
		}
		if m.deleteArm && m.deleteName == "N" {
			m.clearPrompt()
			backup, err := m.svc.BackupCurrentIfUncovered()
			if err != nil {
				m.status = status{statusErr, "自动备份失败: " + err.Error()}
				return m, nil
			}
			if err := antigravity.ClearToken(); err != nil {
				m.status = status{statusErr, "清空凭据失败: " + err.Error()}
				return m, nil
			}
			text := "已登出 agy,可在 agy 里登录新账号后按 s 保存"
			if backup != "" {
				text = "已登出 agy(原账号已自动备份到 " + backup + "),可登录新账号后按 s 保存"
			}
			m.status = status{statusOK, text}
			return m, func() tea.Msg { return aRefreshMsg{} }
		}
		var text string
		if m.svc.CurrentCovered() {
			text = "再按一次 N 登出 agy(当前账号已保存在槽位,服务端会话不会被吊销)"
		} else {
			text = "⚠ 当前账号尚未保存到槽位,再按一次 N 登出(将自动备份到 auto-<时间戳> 槽位)"
		}
		cmd := m.armPrompt("N", text)
		return m, cmd
	case "m", "M":
		m.clearPrompt()
		m.openPanel(modeMachine)
	case "?", "h":
		m.clearPrompt()
		m.openPanel(modeHelp)
	case "enter": // 切换:再按一次 Enter 确认
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		if m.confirmSwitch() {
			backup, err := m.svc.Apply(sel.Name)
			m.setStatus(err, describeApply(sel.Name, backup), "切换失败")
			return m, func() tea.Msg { return aRefreshMsg{} }
		}
		m.armSwitch(sel.Name)
		return m, nil
	default:
		m.clearPrompt()
	}
	return m, nil
}

func (m AntigravityModel) updateInput(msg tea.Msg) (Manager, tea.Cmd) {
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
				if name == "" {
					name = m.defaultSlotName()
				}
				if m.StoreHas(name) {
					m.confirm = aconfirmOverwriteSave
					m.confirmTx = name
					m.mode = modeConfirm
					m.input.Blur()
					return m, statusTick()
				}
				var slot antigravity.Slot
				slot, err = m.svc.SaveCurrent(name, false)
				if err == nil {
					extra = "已保存到槽位 " + describeASlot(slot)
				}
			case inputRename:
				err = m.svc.Store.Rename(m.renameFrom, name)
			}
			if err == nil {
				m.status = status{statusOK, orDefault(extra, "操作成功")}
				m.mode = modeNormal
				m.input.Blur()
				return m, func() tea.Msg { return aRefreshMsg{} }
			}
			m.status = status{statusErr, err.Error()}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m AntigravityModel) updateConfirm(msg tea.Msg) (Manager, tea.Cmd) {
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
		case aconfirmOverwriteSave:
			slot, err := m.svc.SaveCurrent(m.confirmTx, true)
			m.setStatus(err, "已覆盖保存到槽位 "+describeASlot(slot), "保存失败")
			m.mode = modeNormal
			return m, func() tea.Msg { return aRefreshMsg{} }
		}
	}
	return m, nil
}

func (m AntigravityModel) StoreHas(name string) bool { return m.svc.Store.Has(name) }

func describeASlot(slot antigravity.Slot) string {
	email := slot.Email
	if email == "" {
		email = "未知账号"
	}
	return fmt.Sprintf("%q(%s)", slot.Name, email)
}

func describeApply(name, backup string) string {
	text := "已切换到槽位 " + name
	if backup != "" {
		text += "(原账号已自动备份到 " + backup + ")"
	}
	return text
}

// expiryMark 凭据时效的展示文本;没过期且能自动续期时为普通说明。
func expiryMark(state antigravity.TokenState, now time.Time) string {
	switch {
	case state.ExpiresAt == nil:
		if state.Refreshable {
			return "可自动续期"
		}
		return "过期时间未知"
	case state.Expired(now):
		if state.Refreshable {
			return "access_token 已过期(agy 会自动续期)"
		}
		return "已过期,需重新登录"
	default:
		return "有效期至 " + state.ExpiresAt.Local().Format("01-02 15:04")
	}
}

// slotWarn 槽位在列表里的告警标记。
func slotWarn(slot antigravity.Slot, now time.Time) string {
	if slot.State.Expired(now) && !slot.State.Refreshable {
		return styleWarn.Render(" ⚠已过期")
	}
	return ""
}

// ───────────────────────── 视图 ─────────────────────────

func (m AntigravityModel) currentLine() string {
	if !m.current.Exists {
		line := styleWarn.Render("当前账号:未登录 agy(没有找到凭据文件)")
		if m.current.LoggedOut != "" {
			line += styleDim.Render("  · 上次登出的凭据仍保留在同一目录")
		}
		return line
	}
	line := "当前账号:" + stylePlainName.Render(orDefault(m.current.Email, "(未知邮箱)"))
	if m.current.AuthMode != "" {
		line += styleDim.Render("[" + m.current.AuthMode + "]")
	}
	now := time.Now()
	if m.current.State.Expired(now) && !m.current.State.Refreshable {
		line += styleWarn.Render("  ⚠凭据已过期且无 refresh_token,需在 agy 重新登录")
	}
	return line
}

// slotTable 渲染槽位列表表格(表头 + 动态列宽,超出可视高度时滚动)。
func (m AntigravityModel) slotTable() string {
	rows := [][]string{{"  槽位", "邮箱", "最后使用", "状态"}}
	start, end := m.windowRange(len(m.slots), m.cursor)
	now := time.Now()
	for i := start; i < end; i++ {
		slot := m.slots[i]
		inUse := m.current.Exists && antigravity.EqualToken(m.current.Token, slot.Token)

		cursor := "  "
		nameStyle := stylePlainName
		switch {
		case i == m.cursor:
			cursor = styleSel.Bold(true).Render("> ")
			nameStyle = styleSel
		case inUse:
			nameStyle = styleInUseName
		}

		name := slot.Name
		if len(name) > 24 {
			name = name[:24]
		}

		mark := ""
		if inUse {
			mark = badgeInUse()
		}
		if slot.AuthMode != "" && slot.AuthMode != "consumer" {
			mark += styleDim.Render(" " + slot.AuthMode)
		}
		if !slot.State.Refreshable {
			mark += styleWarn.Render(" ⚠无refresh_token")
		}
		mark += slotWarn(slot, now)

		rows = append(rows, []string{
			cursor + nameStyle.Render(name),
			orDefault(slot.Email, "未知账号"),
			shortTime(slot.LastUsedAt),
			mark,
		})
	}
	for i, cell := range rows[0] {
		rows[0][i] = styleDim.Render(cell)
	}
	table := fitTable(rows, m.tableWidth(), []int{2, 1})
	if start > 0 {
		table = styleDim.Render(fmt.Sprintf("↑ 上方还有 %d 个", start)) + "\n" + table
	}
	if end < len(m.slots) {
		table += "\n" + styleDim.Render(fmt.Sprintf("↓ 下方还有 %d 个", len(m.slots)-end))
	}
	return table
}

// panelContent 当前浮层的正文(滚动窗口用,需与 View 里的内容一致)。
func (m AntigravityModel) panelContent() string {
	switch m.mode {
	case modeHelp:
		return aHelpView(m.helpWidth())
	case modeMachine:
		return aInfoView(m)
	}
	return ""
}

func (m AntigravityModel) View() string {
	if m.quit {
		return ""
	}
	w := m.width
	if w <= 0 {
		w = 80
	}

	rowsBlock := "(空)按 s 保存当前 agy 账号到槽位"
	if len(m.slots) > 0 {
		rowsBlock = "\n" + m.slotTable()
	}
	bodyW := max(lipgloss.Width(rowsBlock), 46)
	if bodyW > w-2 {
		bodyW = w - 2
	}

	var body strings.Builder
	body.WriteString(m.currentLine() + "\n")
	if m.agRun {
		body.WriteString(styleWarn.Render("⚠ agy 正在运行:切换后建议尽快重启 agy,避免运行中的凭据回写覆盖") + "\n")
	}
	body.WriteString(styleDim.Render(fmt.Sprintf("槽位(共 %d 个)", len(m.slots))) + "\n")
	body.WriteString(ruleW(bodyW, "238") + "\n")
	body.WriteString(rowsBlock + "\n\n")
	body.WriteString(m.renderStatus() + "\n")

	overlay := ""
	switch m.mode {
	case modeInput:
		purpose := "输入槽位名保存当前账号"
		if m.inputFor == inputRename {
			purpose = "重命名槽位 " + m.renameFrom
		}
		overlay = purpose + "\n" + m.input.View()
	case modeConfirm:
		var text, keys string
		switch m.confirm {
		case aconfirmOverwriteSave:
			text = fmt.Sprintf("槽位 %q 已存在,用当前账号覆盖?", m.confirmTx) + "\n" +
				styleDim.Render("覆盖后原槽位内容不再保留(其他槽位不受影响)")
			keys = "[y] 确认  [n] 取消"
		}
		overlay = styleWarn.Render(text) + "\n" + shakePrefix(m.shakeFrame) + keys
	case modeHelp:
		overlay = aHelpView(m.helpWidth())
	case modeMachine:
		overlay = aInfoView(m)
	}
	if overlay != "" && m.panelOverflow(overlay, aChips) {
		overlay += "\n\n" + styleDim.Render("(↑/↓ 滚动,其他键返回)")
	}

	return m.page(&ainfo, body.String(), overlay, aChips)
}

func aInfoView(m AntigravityModel) string {
	val := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	accentSt := lipgloss.NewStyle().Foreground(ainfo.accent)
	run := styleDim.Render("未检测到")
	if antigravity.Running() {
		run = accentSt.Render("正在运行")
	}
	current := styleDim.Render("(未登录)")
	if m.current.Exists {
		rows := []string{accentSt.Render(orDefault(m.current.Email, "(未知邮箱)"))}
		if m.current.AuthMode != "" {
			rows = append(rows, styleDim.Render("认证方式 "+m.current.AuthMode))
		}
		rows = append(rows, styleDim.Render(expiryMark(m.current.State, time.Now())))
		current = strings.Join(rows, "\n")
	}
	lines := []string{
		styleTitle.Render("信息"),
		styleDim.Render("agy 数据目录: ") + val.Render(antigravity.CLIDir()),
		styleDim.Render("登录凭据:     ") + val.Render(antigravity.TokenPath()),
	}
	parts := strings.Split(current, "\n")
	lines = append(lines, styleDim.Render("当前账号:     ")+parts[0])
	for _, extra := range parts[1:] {
		lines = append(lines, strings.Repeat(" ", len("当前账号:     "))+extra)
	}
	lines = append(lines,
		styleDim.Render("agy:          ")+run,
		styleDim.Render("槽位:         ")+val.Render(antigravity.DefaultDir())+
			accentSt.Render(fmt.Sprintf("  %d 个", len(m.slots))),
		"",
		styleDim.Render("按任意键返回"),
	)
	return strings.Join(lines, "\n")
}

func aHelpView(width int) string {
	return renderHelp("按键说明(agy 账号切换器)", []helpEntry{
		{"j/k 或 ↑/↓", "移动光标"},
		{"←/→", "在底部操作栏中选择操作,回车执行"},
		{"enter", "切换到选中槽位(再按一次 Enter 确认;未保存的当前账号会自动备份)"},
		{"s", "保存当前 agy 账号到新槽位(默认填充邮箱前缀,可编辑)"},
		{"r", "重命名选中槽位"},
		{"d", "删除选中槽位(按两次 d 确认,移入回收站)"},
		{"z", "撤销最近一次删除"},
		{"n", "登出 agy(当前账号自动备份),用于换账号重新登录"},
		{"m", "查看 agy 目录/凭据/账号信息"},
		{"?", "本帮助"},
		{"Esc", "返回目标软件选择"},
		{"q / Q / Ctrl+C", "退出"},
	}, []string{
		styleDim.Render("槽位存储位置:" + antigravity.DefaultDir()),
		styleDim.Render("agy 凭据文件:" + antigravity.TokenPath()),
	}, width)
}
