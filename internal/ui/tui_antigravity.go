// Google Antigravity 账号槽位切换器。
// 实现 Manager 接口接入 agent-maid;共享交互由 baseModel 提供,
// 本文件只包含 Antigravity 自身的数据与动作。
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

// ainfo Antigravity 的展示信息。
var ainfo = targetInfo{
	Key:      "antigravity",
	Name:     "Antigravity",
	Desc:     "Google 账号槽位切换",
	Path:     "~/.gemini/oauth_creds.json",
	pxLarge:  antigravityPxLarge,
	pxSmall:  antigravityPxSmall,
	colLarge: antigravityColLarge,
	colSmall: antigravityColSmall,
	grad:     []string{"#86C561", "#FD553F", "#3087FE"}, // 官方渐变:绿→红→蓝(仅置灰外场景备用)
	accent:   lipgloss.Color("#3087FE"),
}

// aChips Antigravity 界面底部操作栏(与其他工具同一套交互模式)。
var aChips = []chip{
	{"enter", "[↵] 切换"},
	{"s", "[S]ave 保存"},
	{"r", "[R]ename 重命名"},
	{"d", "[D]elete 删除"},
	{"n", "[N]ew 新建"},
	{"m", "[M]eta 信息"},
	{"?", "[?]Help 帮助"},
}

type aconfirm int

const (
	aconfirmOverwriteSave aconfirm = iota
)

type AntigravityModel struct {
	baseModel

	svc        *antigravity.Service
	positioned bool // 首次加载数据后已把光标定位到使用中的账号
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

// NewAntigravity 创建 Antigravity 管理器。
func NewAntigravity() AntigravityModel {
	ti := textinput.New()
	ti.Placeholder = "槽位名,如 work"
	ti.CharLimit = 64
	return AntigravityModel{svc: antigravity.New(), input: ti}
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
				if antigravity.EqualCreds(m.current.Creds, m.slots[i].OAuthCreds) {
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
		m.width = msg.Width
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
		if _, ok := msg.(tea.KeyMsg); ok {
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
		m.status = status{2, msg.err.Error()}
	default:
		text := "已切换到槽位 " + msg.name
		if msg.backup != "" {
			text += "(原账号已自动备份到 " + msg.backup + ")"
		}
		m.status = status{1, text}
	}
	return m, func() tea.Msg { return aRefreshMsg{} }
}

func (m AntigravityModel) updateNormal(msg tea.Msg) (Manager, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "q", "Q":
		return m, m.handleQuitKey()
	case "esc":
		return m, backCmd
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
		if !m.current.Exists {
			m.status = status{2, "没有找到 oauth_creds.json,无账号可保存"}
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
			err := m.svc.Store.Delete(sel.Name)
			m.setStatus(err, "已删除槽位 "+sel.Name, "删除失败")
			return m, func() tea.Msg { return aRefreshMsg{} }
		}
		cmd := m.armPrompt(sel.Name, fmt.Sprintf("再按一次 d 删除槽位 %q", sel.Name))
		return m, cmd
	case "n", "N": // 登出:清空本地凭据(自动备份),让 Antigravity 回到未登录态
		if !m.current.Exists {
			m.status = status{2, "当前没有登录凭据,无需登出"}
			return m, nil
		}
		if m.deleteArm && m.deleteName == "N" {
			m.clearPrompt()
			backup, err := m.svc.BackupCurrentIfUncovered()
			if err != nil {
				m.status = status{2, "自动备份失败: " + err.Error()}
				return m, nil
			}
			if err := antigravity.ClearCreds(); err != nil {
				m.status = status{2, "清空凭据失败: " + err.Error()}
				return m, nil
			}
			text := "已清空本地凭据,可重新登录后按 s 保存新账号"
			if backup != "" {
				text = "已清空本地凭据(原账号已自动备份到 " + backup + "),可重新登录后按 s 保存"
			}
			m.status = status{1, text}
			return m, func() tea.Msg { return aRefreshMsg{} }
		}
		var text string
		if m.svc.CurrentCovered() {
			text = "再按一次 N 清空凭据并登出(当前账号已保存在槽位,令牌不会被吊销)"
		} else {
			text = "⚠ 当前凭据尚未保存到槽位,再按一次 N 登出(将自动备份到 auto-<时间戳> 槽位)"
		}
		cmd := m.armPrompt("N", text)
		return m, cmd
	case "m", "M":
		m.clearPrompt()
		m.mode = modeMachine
	case "?", "h":
		m.clearPrompt()
		m.mode = modeHelp
	case "enter": // 切换:再按一次 Enter 确认(提示在徽章位置)
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
				m.status = status{1, orDefault(extra, "操作成功")}
				m.mode = modeNormal
				m.input.Blur()
				return m, func() tea.Msg { return aRefreshMsg{} }
			}
			m.status = status{2, err.Error()}
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

// ───────────────────────── 视图 ─────────────────────────

func (m AntigravityModel) currentLine() string {
	if !m.current.Exists {
		return styleWarn.Render("当前账号:未找到 oauth_creds.json(尚未登录 Google 账号)")
	}
	line := "当前账号:" + stylePlainName.Render(orDefault(m.current.Email, "(未知)"))
	return line
}

// slotTable 渲染槽位列表表格(表头 + 动态列宽)。
func (m AntigravityModel) slotTable() string {
	rows := [][]string{{"  槽位", "邮箱", "最后使用", "状态"}}
	for i, slot := range m.slots {
		inUse := m.current.Exists && antigravity.EqualCreds(m.current.Creds, slot.OAuthCreds)

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
	return joinTable(rows)
}

func (m AntigravityModel) View() string {
	if m.quit {
		return ""
	}
	w := m.width
	if w <= 0 {
		w = 80
	}

	rowsBlock := "(空)按 s 保存当前 Google 账号,或 S 快速保存"
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
		body.WriteString(styleWarn.Render("⚠ Antigravity 正在运行:切换后建议尽快重启 Antigravity,避免运行中的凭据回写覆盖") + "\n")
	}
	body.WriteString(styleDim.Render(fmt.Sprintf("槽位(共 %d 个)", len(m.slots))) + "\n")
	body.WriteString(ruleW(bodyW, "238") + "\n")
	body.WriteString(rowsBlock + "\n\n")
	body.WriteString(m.renderStatus() + "\n")

	switch m.mode {
	case modeInput:
		purpose := "输入槽位名保存当前账号"
		if m.inputFor == inputRename {
			purpose = "重命名槽位 " + m.renameFrom
		}
		body.WriteString("\n" + styleBox.Render(purpose+"\n"+m.input.View()) + "\n")
	case modeConfirm:
		var text, keys string
		switch m.confirm {
		case aconfirmOverwriteSave:
			text = fmt.Sprintf("槽位 %q 已存在,覆盖?", m.confirmTx)
			keys = "[y] 确认  [n] 取消"
		}
		body.WriteString(m.confirmBox(text, keys))
	case modeHelp:
		body.WriteString("\n" + styleBox.Render(aHelpView()) + "\n")
	case modeMachine:
		body.WriteString("\n" + styleBox.Render(aInfoView(m)) + "\n")
	}

	footer := m.bottomBar(&ainfo, aChips)
	return m.header(&ainfo) + "\n" +
		lipgloss.PlaceHorizontal(w, lipgloss.Center, body.String()) + "\n" +
		lipgloss.PlaceHorizontal(w, lipgloss.Center, footer)
}

func aInfoView(m AntigravityModel) string {
	val := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	accentSt := lipgloss.NewStyle().Foreground(ainfo.accent)
	run := styleDim.Render("未检测到")
	if antigravity.Running() {
		run = accentSt.Render("正在运行")
	}
	return strings.Join([]string{
		styleTitle.Render("信息"),
		styleDim.Render("数据目录:    ") + val.Render(antigravity.GeminiDir()),
		styleDim.Render("凭据文件:    ") + val.Render(antigravity.OAuthCredsPath()),
		styleDim.Render("账号文件:    ") + val.Render(antigravity.AccountsPath()),
		styleDim.Render("当前账号:    ") + val.Render(orDefault(m.current.Email, "(未登录)")),
		styleDim.Render("Antigravity: ") + run,
		styleDim.Render("槽位:        ") + val.Render(antigravity.DefaultDir()) +
			accentSt.Render(fmt.Sprintf("  %d 个", len(m.slots))),
		"",
		styleDim.Render("按任意键返回"),
	}, "\n")
}

func aHelpView() string {
	return strings.Join([]string{
		styleTitle.Render("按键说明(Antigravity 账号切换器)"),
		"j/k 或 ↑/↓   移动光标",
		"←/→          在底部操作栏中选择操作,回车执行",
		"enter        切换到选中槽位(再按一次 Enter 确认;自动备份未覆盖的当前账号,按偏好自动重启)",
		"s            保存当前账号到新槽位(默认填充邮箱前缀,可编辑)",
		"S            快速保存当前账号(自动命名)",
		"r            重命名选中槽位",
		"d            删除选中槽位(按两次 d 确认,5s 不动自动撤下)",
		"m            查看数据目录/账号信息",
		"?            本帮助",
		"Esc          返回目标软件选择",
		"q / Q / Ctrl+C 退出",
		"",
		styleDim.Render("槽位存储位置:" + antigravity.DefaultDir()),
		styleDim.Render("凭据文件:" + antigravity.OAuthCredsPath()),
		"",
		styleDim.Render("按任意键返回"),
	}, "\n")
}
