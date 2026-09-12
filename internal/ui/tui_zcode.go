// ZCode 账号槽位切换器(业务主体复用自 zcode-acc-switch)。
// 实现 Manager 接口接入 agent-maid;共享交互(操作栏/删除待确认/摇晃动画)
// 由 baseModel 提供,本文件只包含 ZCode 自身的数据与动作。
package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"agent-maid/internal/background"
	"agent-maid/internal/zcode"
	"agent-maid/internal/zcodeapp"
	"agent-maid/internal/zcodestore"
)

// zinfo ZCode 的展示信息。
var zinfo = targetInfo{
	Key:     "zcode",
	Name:    "ZCode",
	Desc:    "桌面端账号槽位切换",
	Path:    "~/.zcode/v2/credentials.json",
	pxLarge: zcodePxLarge,
	pxSmall: zcodePxSmall,
	grad:    []string{"#F472B6", "#A855F7", "#6366F1"},
	accent:  lipgloss.Color("212"),
}

// zChips ZCode 界面底部操作栏(与 ChatGPT 界面同一套交互模式)。
var zChips = []chip{
	{"enter", "[↵] 切换"},
	{"s", "[S]ave 保存"},
	{"r", "[R]ename 重命名"},
	{"d", "[D]elete 删除"},
	{"n", "[N]ew 新建"},
	{"m", "[M]achine 信息"},
	{"?", "[?]Help 帮助"},
}

type zconfirm int

const (
	zconfirmOverwriteSave zconfirm = iota
)

type ZcodeModel struct {
	baseModel

	svc        *zcodeapp.Service
	positioned bool // 首次加载数据后已把光标定位到使用中的账号
	slots      []zcodestore.Slot
	cursor     int
	current    zcodeapp.CurrentInfo
	zcodeRun   bool

	input      textinput.Model
	inputFor   inputPurpose
	renameFrom string

	confirm   zconfirm
	confirmTx string // 确认附带的上下文(槽位名)

	waitingFor string
	waitProcs  []zcode.Proc
	waitStart  time.Time
	spin       int // 等待界面 spinner 帧序号
}

// NewZcode 创建 ZCode 管理器。
func NewZcode() ZcodeModel {
	ti := textinput.New()
	ti.Placeholder = "槽位名,如 work"
	ti.CharLimit = 64
	return ZcodeModel{svc: zcodeapp.New(), input: ti}
}

// Info 实现 Manager。
func (m ZcodeModel) Info() *targetInfo { return &zinfo }

// Chips 实现 Manager。
func (m ZcodeModel) Chips() []chip { return zChips }

func (m ZcodeModel) Init() tea.Cmd { return m.refresh }

// zcodeAccountLabel 渲染 ZCode 账号摘要(provider · 用户名)。
func zcodeAccountLabel(meta zcode.AccountMeta) string {
	parts := []string{}
	if meta.Provider != "" {
		parts = append(parts, meta.Provider)
	}
	name := meta.DisplayName
	if name == "" {
		name = meta.Username
	}
	if name != "" {
		parts = append(parts, name)
	}
	if len(parts) == 0 {
		return "未知账号(密钥不匹配或凭据为空)"
	}
	return strings.Join(parts, " / ")
}

// ───────────────────────── 数据 ─────────────────────────

type zLoadedMsg struct {
	slots    []zcodestore.Slot
	current  zcodeapp.CurrentInfo
	zcodeRun bool
}

// waitTickMsg 等待 ZCode 退出的轮询节拍(兼作 spinner 动画帧)。
type waitTickMsg struct{}

// spinFramesBraille 等待界面的 braille 菊花动画帧。
var spinFramesBraille = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinFrames 依据向导结果选择动画帧(纯文本模式用 ASCII)。
func spinFrames() []string {
	if plainMode {
		return []string{"|", "/", "-", "\\"}
	}
	return spinFramesBraille
}

// zApplyResultMsg 异步执行切换的结果。
type zApplyResultMsg struct {
	name   string
	backup string
	err    error
	waited bool // 是否经历了"等待 ZCode 退出"
	forced bool // 是否为无视风险的立即替换
}

func waitTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return waitTickMsg{} })
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
	// 首次加载数据时,把光标定位到使用中的账号
	if !m.positioned {
		m.positioned = true
		if m.current.Exists {
			for i := range m.slots {
				if zcode.EqualCredentials(m.current.Creds, m.slots[i].Credentials) {
					m.cursor = i
					break
				}
			}
		}
	}
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

// defaultSlotName 用当前账号生成默认槽位名(provider-用户名),无账号信息时用时间戳。
func (m ZcodeModel) defaultSlotName() string {
	meta := m.current.Meta
	name := meta.DisplayName
	if name == "" {
		name = meta.Username
	}
	switch {
	case meta.Provider != "" && name != "":
		return meta.Provider + "-" + name
	case name != "":
		return name
	case meta.Provider != "":
		return meta.Provider
	}
	return time.Now().Format("snapshot-0102-150405")
}

// ───────────────────────── 更新 ─────────────────────────

type zRefreshMsg struct{}

func (m ZcodeModel) Update(msg tea.Msg) (Manager, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case zRefreshMsg:
		return m, m.refresh
	case zLoadedMsg:
		m.applyLoaded(msg)
		return m, nil
	case zApplyResultMsg:
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
func (m ZcodeModel) handleApplyResult(msg zApplyResultMsg) (Manager, tea.Cmd) {
	m.mode = modeNormal
	switch {
	case msg.err != nil:
		m.status = status{2, msg.err.Error()}
		return m, func() tea.Msg { return zRefreshMsg{} }
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

func (m ZcodeModel) updateNormal(msg tea.Msg) (Manager, tea.Cmd) {
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
		m.chipNav(-1, len(zChips))
		return m, nil
	case "right":
		m.chipNav(1, len(zChips))
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
		// 回车执行操作栏当前选中项;默认第 0 项即"切换"
		return m.dispatch(zChips[m.chipIdx].key)
	default:
		return m.dispatch(key.String())
	}
	return m, nil
}

// dispatch 统一的动作分发:字母键与操作栏回车共用。
func (m ZcodeModel) dispatch(key string) (Manager, tea.Cmd) {
	switch key {
	case "s", "S": // 保存当前账号
		if !m.current.Exists {
			m.status = status{2, "没有找到 credentials.json,无账号可保存"}
			return m, nil
		}
		m.clearPrompt()
		m.mode = modeInput
		m.inputFor = inputSave
		m.input.SetValue(m.defaultSlotName()) // 默认填充,可编辑
		m.input.CursorEnd()
		m.input.Focus()
		return m, textinput.Blink
	case "r": // 重命名
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
	case "d": // 删除:再按一次 d 确认,5s 不动自动撤下
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		if m.deleteArm && m.deleteName == sel.Name {
			m.clearPrompt()
			err := m.svc.Store.Delete(sel.Name)
			m.setStatus(err, "已删除槽位 "+sel.Name, "删除失败")
			return m, func() tea.Msg { return zRefreshMsg{} }
		}
		cmd := m.armPrompt(sel.Name, fmt.Sprintf("再按一次 d 删除槽位 %q", sel.Name))
		return m, cmd
	case "n", "N": // 登出:清空本地凭据(自动备份),让 ZCode 回到未登录态
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
			if err := zcode.ClearCredentials(); err != nil {
				m.status = status{2, "清空凭据失败: " + err.Error()}
				return m, nil
			}
			text := "已清空本地凭据,可重新登录后按 s 保存新账号"
			if backup != "" {
				text = "已清空本地凭据(原账号已自动备份到 " + backup + "),可重新登录后按 s 保存"
			}
			m.status = status{1, text}
			return m, func() tea.Msg { return zRefreshMsg{} }
		}
		var text string
		if m.svc.CurrentCovered() {
			text = "再按一次 N 清空凭据并登出(当前账号已保存在槽位,令牌不会被吊销)"
		} else {
			text = "⚠ 当前凭据尚未保存到槽位,再按一次 N 登出(将自动备份到 auto-<时间戳> 槽位)"
		}
		cmd := m.armPrompt("N", text)
		return m, cmd
	case "m":
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
		if !m.confirmSwitch() {
			m.armSwitch(sel.Name)
			return m, nil
		}
		if m.zcodeRun {
			// 默认路径:等待 ZCode 完全退出后再执行替换
			m.waitingFor = sel.Name
			m.waitProcs = zcode.ZCodeProcs()
			m.waitStart = time.Now()
			m.mode = modeWaiting
			return m, waitTick()
		}
		return m, m.applyCmd(sel.Name, false, false)
	default:
		m.clearPrompt()
	}
	return m, nil
}

func (m ZcodeModel) updateInput(msg tea.Msg) (Manager, tea.Cmd) {
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
					name = m.defaultSlotName() // 清空后回车也用默认名
				}
				if m.StoreHas(name) {
					// 已存在:转入覆盖确认,避免静默覆盖
					m.confirm = zconfirmOverwriteSave
					m.confirmTx = name
					m.mode = modeConfirm
					m.input.Blur()
					return m, statusTick()
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

func (m ZcodeModel) updateConfirm(msg tea.Msg) (Manager, tea.Cmd) {
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
		case zconfirmOverwriteSave:
			slot, err := m.svc.SaveCurrent(m.confirmTx, true)
			m.setStatus(err, "已覆盖保存到槽位 "+describeSlot(slot), "保存失败")
			m.mode = modeNormal
			return m, func() tea.Msg { return zRefreshMsg{} }
		}
	}
	return m, nil
}

func (m ZcodeModel) updateWaiting(msg tea.Msg) (Manager, tea.Cmd) {
	if _, ok := msg.(waitTickMsg); ok {
		m.spin++
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
	case "c":
		// 关闭本工具,派生后台进程继续等待 ZCode 退出并完成切换
		if err := background.SpawnWaitZcode(m.waitingFor); err != nil {
			m.status = status{2, "后台任务启动失败: " + err.Error()}
			return m, nil
		}
		m.quit = true
		return m, tea.Quit
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "q", "Q":
		return m, m.handleQuitKey()
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

// slotTable 渲染槽位列表表格(表头 + 动态列宽)。
func (m ZcodeModel) slotTable() string {
	rows := [][]string{{"  槽位", "账号", "最后使用", "状态"}}
	for i, slot := range m.slots {
		inUse := m.current.Exists && zcode.EqualCredentials(m.current.Creds, slot.Credentials)

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
		if slot.KeyFingerprint != zcode.KeyFingerprint(zcode.DeriveKey(zcode.CredentialSecret())) {
			mark += styleWarn.Render(" ⚠跨机/跨密钥")
		}

		rows = append(rows, []string{
			cursor + nameStyle.Render(name),
			zcodeAccountLabel(slot.Meta),
			shortTime(slot.LastUsedAt),
			mark,
		})
	}
	for i, cell := range rows[0] {
		rows[0][i] = styleDim.Render(cell)
	}
	return joinTable(rows)
}

func (m ZcodeModel) View() string {
	if m.quit {
		return ""
	}
	w := m.width
	if w <= 0 {
		w = 80
	}

	// 列表先行,正文块宽度按内容计算,整体在终端内水平居中
	rowsBlock := "(空)按 s 保存当前账号,或 S 快速保存"
	if len(m.slots) > 0 {
		rowsBlock = "\n" + m.slotTable()
	}
	bodyW := max(lipgloss.Width(rowsBlock), 46)
	if bodyW > w-2 {
		bodyW = w - 2
	}

	var body strings.Builder
	body.WriteString(m.currentLine() + "\n")
	if m.zcodeRun {
		body.WriteString(styleWarn.Render("⚠ ZCode 正在运行:切换将默认等待其完全退出后执行,确认时也可选择无视风险立即替换") + "\n")
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
		case zconfirmOverwriteSave:
			text = fmt.Sprintf("槽位 %q 已存在,覆盖?", m.confirmTx)
			keys = "[y] 确认  [n] 取消"
		}
		body.WriteString(m.confirmBox(text, keys))
	case modeWaiting:
		elapsed := time.Since(m.waitStart).Truncate(time.Second)
		spin := styleSel.Render(spinFrames()[m.spin%len(spinFrames())])
		lines := []string{
			styleTitle.Render(fmt.Sprintf("%s 等待 ZCode 完全退出…(已等待 %v)", spin, elapsed)),
			fmt.Sprintf("ZCode 退出后将自动切换到槽位 %q 并写入 ZCode 的凭据。", m.waitingFor),
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
		lines = append(lines, "", styleWarn.Render("[f] 立即替换(无视风险)")+"  [c] 关闭工具,后台继续等待并切换  [n] 取消等待  [q] 退出")
		body.WriteString("\n" + styleBox.Render(strings.Join(lines, "\n")) + "\n")
	case modeHelp:
		body.WriteString("\n" + styleBox.Render(zHelpView()) + "\n")
	case modeMachine:
		body.WriteString("\n" + styleBox.Render(zMachineView()) + "\n")
	}

	footer := m.bottomBar(&zinfo, zChips)
	return m.header(&zinfo) + "\n" +
		centerBlock(body.String(), w) + "\n" +
		centerBlock(footer, w)
}

func zMachineView() string {
	info := zcode.GetMachineInfo()
	val := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	accentSt := lipgloss.NewStyle().Foreground(zinfo.accent)
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
	return strings.Join(lines, "\n")
}

func zHelpView() string {
	return strings.Join([]string{
		styleTitle.Render("按键说明(ZCode 账号切换器)"),
		"j/k 或 ↑/↓   移动光标",
		"←/→          在底部操作栏中选择操作,回车执行",
		"enter        切换到选中槽位(再按一次 Enter 确认;自动备份未覆盖的当前账号)",
		styleDim.Render("             ZCode 运行中时,默认等待其完全退出后自动切换;"),
		styleDim.Render("             确认/等待界面按 f 可无视风险立即替换。"),
		"s            保存当前账号到新槽位(默认填充账号名,可编辑)",
		"r            重命名选中槽位",
		"d            删除选中槽位(按两次 d 确认,5s 不动自动撤下)",
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
