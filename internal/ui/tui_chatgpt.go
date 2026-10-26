// ChatGPT(Codex)账号库切换器。
// 实现 Manager 接口接入 agent-maid;共享交互(操作栏/删除待确认/摇晃动画)
// 由 baseModel 提供,本文件只包含 ChatGPT 自身的数据与动作。
package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"agent-maid/internal/gptapp"
	"agent-maid/internal/gptauth"
	"agent-maid/internal/gptstore"
)

// ───────────────────────── 模型 ─────────────────────────

// cinfo ChatGPT 的展示信息。
var cinfo = targetInfo{
	Key:     "chatgpt",
	Name:    "ChatGPT",
	Desc:    "Codex 登录账号切换",
	Path:    "~/.codex/auth.json",
	pxLarge: chatgptPxLarge,
	pxSmall: chatgptPxSmall,
	grad:    []string{"#5EEAD4", "#10A37F", "#0EA5E9"},
	accent:  lipgloss.Color("#10A37F"),
}

// cChips ChatGPT 界面底部操作栏(与 ZCode 界面同一套交互模式)。
var cChips = []chip{
	{"enter", "[↵] 切换"},
	{"s", "[S] 保存"},
	{"i", "[I] 导入"},
	{"r", "[R] 重命名"},
	{"d", "[D] 删除"},
	{"z", "[Z] 撤销"},
	{"n", "[N] 登出"},
	{"m", "[M] 信息"},
	{"?", "[?] 帮助"},
}

type ChatgptModel struct {
	baseModel

	loadErr    error
	positioned bool     // 首次加载数据后已把光标定位到使用中的账号
	trashStack []string // 回收站文件名栈(撤销用)

	accounts []*gptstore.Account
	activeID string
	current  *gptauth.Current
	home     string
	codexRun bool

	cursor int

	input    textinput.Model
	inputFor inputPurpose
}

// NewChatgpt 创建 ChatGPT 管理器。
func NewChatgpt() ChatgptModel {
	purgeGptTrash()
	ti := textinput.New()
	ti.Placeholder = "JSON 文件路径,或直接粘贴 JSON"
	ti.CharLimit = 0
	return ChatgptModel{input: ti}
}

// Info 实现 Manager。
func (m ChatgptModel) Info() *targetInfo { return &cinfo }

// Chips 实现 Manager。
func (m ChatgptModel) Chips() []chip { return cChips }

func (m ChatgptModel) Init() tea.Cmd { return m.refresh }

type cLoadedMsg struct {
	accounts []*gptstore.Account
	activeID string
	current  *gptauth.Current
	home     string
	codexRun bool
	err      error
}

// purgeGptTrash 清空账号回收站(正式删除,每次启动执行一次)。
func purgeGptTrash() {
	if st, err := gptstore.Load(gptstore.DefaultPath()); err == nil {
		_ = st.PurgeTrash()
	}
}

func (m ChatgptModel) refresh() tea.Msg {
	st, err := gptstore.Load(gptstore.DefaultPath())
	if err != nil {
		return cLoadedMsg{err: err}
	}
	home, err := gptauth.ResolveHome("")
	if err != nil {
		return cLoadedMsg{err: err}
	}
	current, err := gptauth.ReadCurrent(home)
	if err != nil {
		return cLoadedMsg{err: err, home: home}
	}
	return cLoadedMsg{accounts: st.Data.Accounts, activeID: st.Data.ActiveID, current: current, home: home, codexRun: gptauth.CodexRunning()}
}

func (m *ChatgptModel) applyLoaded(lm cLoadedMsg) {
	m.loadErr = lm.err
	m.home = lm.home
	m.accounts = lm.accounts
	m.activeID = lm.activeID
	m.current = lm.current
	m.codexRun = lm.codexRun
	m.clampCursor()
}

func (m *ChatgptModel) clampCursor() {
	if m.cursor >= len(m.accounts) {
		m.cursor = len(m.accounts) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m ChatgptModel) selected() *gptstore.Account {
	if m.cursor >= 0 && m.cursor < len(m.accounts) {
		return m.accounts[m.cursor]
	}
	return nil
}

// ───────────────────────── 更新 ─────────────────────────

type cRefreshMsg struct{}

func (m ChatgptModel) Update(msg tea.Msg) (Manager, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case cRefreshMsg:
		return m, m.refresh
	case cLoadedMsg:
		m.applyLoaded(msg)
		return m, nil
	case statusTickMsg:
		cmd := m.handleStatusTick()
		return m, cmd
	}

	// 页面主体
	switch m.mode {
	case modeInput:
		return m.updateInput(msg)
	case modeHelp, modeDetail, modeMachine:
		if key, ok := msg.(tea.KeyMsg); ok {
			if m.scrollPanel(key.String(), m.panelContent(), cChips) {
				return m, nil
			}
			m.mode = modeNormal
		}
		return m, nil
	}
	return m.updateNormal(msg)
}

// reload 同步重新读盘(当前凭据/账号库/运行状态)。
func (m *ChatgptModel) reload() {
	if lm, ok := m.refresh().(cLoadedMsg); ok {
		m.applyLoaded(lm)
	}
}

func (m ChatgptModel) updateNormal(msg tea.Msg) (Manager, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		m.quit = true
		purgeGptTrash()
		return m, tea.Quit
	case "q", "Q":
		cmd := m.handleQuitKey()
		if m.quit {
			purgeGptTrash()
		}
		return m, cmd
	case "esc":
		return m, backCmd
	case "ctrl+r":
		m.clearPrompt()
		m.reload()
		m.status = status{statusOK, "已重新读取当前凭据与账号库"}
		return m, nil
	case "left":
		m.chipNav(-1, len(cChips))
		return m, nil
	case "right":
		m.chipNav(1, len(cChips))
		return m, nil
	case "up", "k":
		m.clearPrompt()
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		m.clearPrompt()
		if m.cursor < len(m.accounts)-1 {
			m.cursor++
		}
	case "g", "home":
		m.clearPrompt()
		m.cursor = 0
	case "G", "end":
		m.clearPrompt()
		m.cursor = len(m.accounts) - 1
	case "enter":
		return m.dispatch(cChips[m.chipIdx].key)
	default:
		return m.dispatch(key.String())
	}
	return m, nil
}

// dispatch 统一的动作分发:字母键与操作栏回车共用(字母大小写不区分)。
func (m ChatgptModel) dispatch(key string) (Manager, tea.Cmd) {
	switch key {
	case "i", "I": // 导入 JSON
		m.clearPrompt()
		m.mode = modeInput
		m.inputFor = inputImport
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	case "s", "S": // 保存当前凭据到账号库(输入显示名)
		m.reload() // 官方端可能刚登录/续期过,先读最新 auth.json
		m.clearPrompt()
		m.mode = modeInput
		m.inputFor = inputGSave
		m.input.SetValue("")
		m.input.Placeholder = "账号显示名(回车用默认:" + m.currentLabel() + ")"
		m.input.Focus()
		return m, textinput.Blink
	case "r", "R": // 重命名选中账号
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		m.clearPrompt()
		m.mode = modeInput
		m.inputFor = inputGRename
		m.input.SetValue(sel.Label)
		m.input.Placeholder = "账号显示名"
		m.input.Focus()
		return m, textinput.Blink
	case "d", "D": // 删除:再按一次 d 确认,5s 不动自动撤下
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		if m.deleteArm && m.deleteName == sel.ID {
			m.clearPrompt()
			st, lerr := gptstore.Load(gptstore.DefaultPath())
			if lerr != nil {
				m.status = status{statusErr, "读取账号库失败: " + lerr.Error()}
				return m, nil
			}
			trashName, err := st.DeleteToTrash(sel.ID)
			if err == nil {
				m.trashStack = append(m.trashStack, trashName)
				m.setStatus(nil, "已移除账号 "+sel.Label+"(移入回收站,退出前可按 z 撤销)", "")
			} else {
				m.setStatus(err, "", "移除失败")
			}
			return m, func() tea.Msg { return cRefreshMsg{} }
		}
		cmd := m.armPrompt(sel.ID, fmt.Sprintf("再按一次 d 删除账号 %q", sel.Label))
		return m, cmd
	case "z", "Z": // 撤销:从回收站恢复最近删除的账号
		st, lerr := gptstore.Load(gptstore.DefaultPath())
		if lerr != nil {
			m.status = status{statusErr, "读取账号库失败: " + lerr.Error()}
			return m, nil
		}
		name, err := st.RestoreLatestFromTrash()
		if err != nil {
			m.status = status{statusWarn, "没有可撤销的删除"}
			return m, nil
		}
		if n := len(m.trashStack); n > 0 {
			m.trashStack = m.trashStack[:n-1]
		}
		m.status = status{statusOK, "已恢复账号 " + name}
		return m, func() tea.Msg { return cRefreshMsg{} }
	case "n", "N": // 登出:清空本地 auth.json(账号保留在账号库)
		if m.current == nil {
			m.status = status{statusErr, "当前未登录,无需登出"}
			return m, nil
		}
		if m.deleteArm && m.deleteName == "N" {
			m.clearPrompt()
			if _, err := m.saveCurrent(""); err != nil {
				m.status = status{statusErr, "保存当前凭据失败: " + err.Error()}
				return m, nil
			}
			if err := gptauth.ClearAuthFile(m.home); err != nil {
				m.status = status{statusErr, "清空凭据失败: " + err.Error()}
				return m, nil
			}
			m.status = status{statusOK, "已登出 Codex(账号已保留在账号库),重新登录后按 s 保存"}
			return m, func() tea.Msg { return cRefreshMsg{} }
		}
		var text string
		if m.currentInLibrary() {
			text = "再按一次 N 清空凭据并登出(账号已保留在账号库,令牌不会被吊销)"
		} else {
			text = "⚠ 当前凭据尚未保存到账号库,再按一次 N 登出(将自动存入账号库)"
		}
		cmd := m.armPrompt("N", text)
		return m, cmd
	case "m", "M":
		m.clearPrompt()
		m.openPanel(modeMachine)
	case "?", "h":
		m.clearPrompt()
		m.openPanel(modeHelp)
	case "enter": // 激活:再按一次 Enter 确认(提示在徽章位置)
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		if m.confirmSwitch() {
			text, err := m.doActivate(sel.ID)
			m.setStatus(err, text, "激活失败")
			return m, func() tea.Msg { return cRefreshMsg{} }
		}
		m.armSwitch(sel.ID)
		return m, nil
	default:
		m.clearPrompt()
	}
	return m, nil
}

// currentInLibrary 当前 auth.json 的账号是否已存在于账号库。
func (m ChatgptModel) currentInLibrary() bool {
	if m.current == nil || m.current.AccountID == "" {
		return false
	}
	for _, a := range m.accounts {
		if a.AccountID == m.current.AccountID {
			return true
		}
	}
	return false
}

// currentLabel 当前 auth.json 凭据的默认显示名。
func (m ChatgptModel) currentLabel() string {
	if m.current == nil {
		return "auth.json"
	}
	if m.current.Email != "" {
		return m.current.Email
	}
	if m.current.AccountID != "" {
		return m.current.AccountID
	}
	return "auth.json"
}

func (m ChatgptModel) updateInput(msg tea.Msg) (Manager, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if ok {
		switch key.String() {
		case "esc":
			m.mode = modeNormal
			m.input.Blur()
			return m, nil
		case "enter":
			raw := strings.TrimSpace(m.input.Value())
			switch m.inputFor {
			case inputImport:
				if raw == "" {
					m.mode = modeNormal
					m.input.Blur()
					return m, nil
				}
				m.status = m.importAccounts(raw)
			case inputGSave:
				text, err := m.saveCurrent(raw)
				m.setStatus(err, text, "保存失败")
			case inputGRename:
				if sel := m.selected(); sel != nil && raw != "" {
					err := m.renameAccount(sel.ID, raw)
					m.setStatus(err, "已重命名为 "+raw, "重命名失败")
				}
			}
			m.mode = modeNormal
			m.input.Blur()
			return m, func() tea.Msg { return cRefreshMsg{} }
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// saveCurrent 把官方 auth.json 里当前生效的凭据存入账号库。
// label 为空时用 email / account_id 自动命名。
func (m *ChatgptModel) saveCurrent(label string) (string, error) {
	res, err := gptapp.New().SaveCurrent(label)
	if err != nil {
		return "", err
	}
	verb := "已保存当前凭据到账号库"
	if res.Updated {
		verb = "凭据已在账号库中,已更新"
	}
	return fmt.Sprintf("%s:%s", verb, res.Account.Label), nil
}

// renameAccount 改账号库里的显示名。
func (m *ChatgptModel) renameAccount(id, label string) error {
	_, err := gptapp.New().Rename(id, label)
	return err
}

// importAccounts 解析输入(文件路径或粘贴的 JSON),转换后入库。不自动激活。
func (m *ChatgptModel) importAccounts(raw string) status {
	var data []byte
	src := "pasted-json"
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		data = []byte(raw)
	} else {
		b, err := os.ReadFile(gptapp.ExpandHome(raw))
		if err != nil {
			return status{statusErr, "读取文件失败: " + err.Error()}
		}
		data = b
		src = filepath.Base(gptapp.ExpandHome(raw))
	}
	res, err := gptapp.New().Import(data, src)
	switch {
	case errors.Is(err, gptapp.ErrNoAccounts):
		msg := err.Error()
		if res.BadDocs > 0 {
			msg += fmt.Sprintf("(另有 %d 个无效文档)", res.BadDocs)
		}
		return status{statusErr, msg}
	case errors.Is(err, gptapp.ErrConvertFailed):
		msg := fmt.Sprintf("全部 %d 项均无法转换", res.Total)
		if res.BadDocs > 0 {
			msg += fmt.Sprintf("(另有 %d 个无效文档)", res.BadDocs)
		}
		return status{statusErr, msg}
	case err != nil:
		return status{statusErr, err.Error()}
	}
	tail := ""
	if res.Skipped > 0 {
		tail += fmt.Sprintf(",跳过 %d 项", res.Skipped)
	}
	if res.BadDocs > 0 {
		tail += fmt.Sprintf(",忽略 %d 个无效文档", res.BadDocs)
	}
	if res.Imported == 1 {
		return status{statusOK, fmt.Sprintf("已导入 %s%s,Enter 激活并写入官方目录", res.First, tail)}
	}
	return status{statusOK, fmt.Sprintf("已导入 %d 个账号%s,选中后按 Enter 激活", res.Imported, tail)}
}

// doActivate 把账号投影为官方 auth.json(保序合并、原子写入)。
func (m *ChatgptModel) doActivate(id string) (string, error) {
	res, err := gptapp.New().Activate(id)
	if err != nil {
		return "", err
	}
	return "已激活 " + res.Account.Label + ",已写入 Codex 凭据(" + res.Path + ")", nil
}

// currentLine 顶部当前凭据状态行。
func (m ChatgptModel) currentLine() string {
	if m.loadErr != nil {
		return styleErr.Render("加载失败:" + m.loadErr.Error())
	}
	if m.current == nil {
		return styleWarn.Render("当前凭据:auth.json 不存在(官方端尚未登录)")
	}

	mode := prettyAuthMode(m.current.AuthMode)
	email := orDash(m.current.Email)
	id := orDash(m.current.AccountID)
	refresh := "—"
	if m.current.LastRefresh != "" {
		refresh = refreshText(m.current.LastRefresh)
	}

	var b strings.Builder
	b.WriteString("当前凭据:" + prettyColored(m.current.AuthMode != "", mode) + " | " + email + "\n")
	if m.current.AccountID != "" {
		b.WriteString("账号 ID:" + id + "\n")
	}
	b.WriteString("最近刷新:" + styleDim.Render(refresh))
	return b.String()
}

// prettyColored 条件高亮:ok 为真时用强调色渲染文本。
func prettyColored(ok bool, s string) string {
	if ok {
		return accentStyle.Render(s)
	}
	return s
}

// accentStyle 平台强调色(取当前工具的 accent)。
var accentStyle = lipgloss.NewStyle().Foreground(cinfo.accent)

// accountTable 渲染账号列表表格(表头 + 动态列宽,超出可视高度时滚动)。
func (m ChatgptModel) accountTable() string {
	rows := [][]string{{"  账号", "套餐", "来源", "ID", "最后使用", "状态"}}
	start, end := m.windowRange(len(m.accounts), m.cursor)
	for i := start; i < end; i++ {
		a := m.accounts[i]
		inUse := m.current != nil && a.AccountID != "" && a.AccountID == m.current.AccountID

		cursor := "  "
		nameStyle := stylePlainName
		switch {
		case i == m.cursor:
			cursor = styleSel.Bold(true).Render("> ")
			nameStyle = styleSel
		case inUse:
			nameStyle = styleInUseName
		}

		name := a.Label
		if len(name) > 30 {
			name = name[:30]
		}

		mark := ""
		if inUse {
			mark = badgeInUse()
		}
		if !a.Refreshable {
			mark += styleWarn.Render(" ⚠无refresh_token")
		}
		if a.AccountID == "" {
			mark += styleWarn.Render(" ⚠缺account_id")
		}

		rows = append(rows, []string{
			cursor + nameStyle.Render(name),
			orDash(a.PlanType),
			"[" + a.SourceFormat + "]",
			shortID8(a.AccountID),
			shortTime(a.LastUsedAt),
			mark,
		})
	}
	for i, cell := range rows[0] {
		rows[0][i] = styleDim.Render(cell)
	}
	// 窄终端依次丢弃 来源→ID→最后使用→套餐,保持列宽恒定不折行
	table := fitTable(rows, m.tableWidth(), []int{2, 3, 4, 1})
	if start > 0 {
		table = styleDim.Render(fmt.Sprintf("↑ 上方还有 %d 个", start)) + "\n" + table
	}
	if end < len(m.accounts) {
		table += "\n" + styleDim.Render(fmt.Sprintf("↓ 下方还有 %d 个", len(m.accounts)-end))
	}
	return table
}

// panelContent 当前浮层的正文(滚动窗口用,需与 View 里的内容一致)。
func (m ChatgptModel) panelContent() string {
	switch m.mode {
	case modeHelp:
		return cHelpView(m.helpWidth())
	case modeMachine:
		return m.infoView()
	case modeDetail:
		if sel := m.selected(); sel != nil {
			return m.detailView(sel)
		}
	}
	return ""
}

func (m ChatgptModel) View() string {
	if m.quit {
		return ""
	}
	w := m.width
	if w <= 0 {
		w = 80
	}

	var rowLines []string
	if len(m.accounts) == 0 {
		rowLines = append(rowLines, styleDim.Render("(空)按 i 导入 WebSession / sub2api 导出的 JSON,或 s 保存当前登录凭据"))
	} else {
		rowLines = append(rowLines, m.accountTable())
	}
	rowsBlock := strings.Join(rowLines, "\n")
	bodyW := max(lipgloss.Width(rowsBlock), 56)
	if bodyW > w-2 {
		bodyW = w - 2
	}

	var body strings.Builder
	body.WriteString(m.currentLine() + "\n")
	if m.codexRun {
		body.WriteString(styleWarn.Render("⚠ Codex 正在运行:切换后建议尽快重启 Codex,避免运行中的凭据回写覆盖") + "\n")
	}
	body.WriteString(styleDim.Render(fmt.Sprintf("账号库(共 %d 个)", len(m.accounts))) + "\n")
	body.WriteString(ruleW(bodyW, "238") + "\n")
	body.WriteString(rowsBlock + "\n\n")
	body.WriteString(m.renderStatus() + "\n")

	overlay := ""
	switch m.mode {
	case modeInput:
		purpose := "导入 ChatGPT 会话凭据(WebSession / sub2api)"
		if m.inputFor == inputGSave {
			purpose = "保存当前凭据到账号库"
		} else if m.inputFor == inputGRename {
			purpose = "重命名账号"
		}
		overlay = purpose + "\n" + m.input.View()
	case modeDetail:
		if sel := m.selected(); sel != nil {
			overlay = m.detailView(sel)
		}
	case modeHelp:
		overlay = cHelpView(m.helpWidth())
	case modeMachine:
		overlay = m.infoView()
	}
	if overlay != "" && m.panelOverflow(overlay, cChips) {
		overlay += "\n\n" + styleDim.Render("(↑/↓ 滚动,其他键返回)")
	}

	return m.page(&cinfo, body.String(), overlay, cChips)
}

// prettyAuthMode 认证模式的展示名(官方值为小写,展示遵循大小写)。
func prettyAuthMode(mode string) string {
	switch mode {
	case "chatgpt":
		return "ChatGPT"
	case "apikey":
		return "API Key"
	}
	return mode
}

// refreshText 把 last_refresh 的时间戳转成本地短格式,解析失败原样返回。
func refreshText(raw string) string {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return raw
}

// mask 让 token 不进终端回滚区:只留前缀与长度。
func mask(token string) string {
	if token == "" {
		return "(空)"
	}
	preview := 12
	if len(token) < preview {
		preview = len(token)
	}
	return token[:preview] + "…(共 " + strconv.Itoa(len(token)) + " 字符)"
}

// expandHome 展开 ~ 前缀。
func expandHome(p string) string {
	if p == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// parseJSONDocs 解析导入输入,尽量宽容:
//  1. 连续 JSON 流:单对象/数组、CPA、sub2api 整包、拼接 JSON、以及
//     "空格分隔的多个 JSON"都算(从终端粘贴多行 JSONL 时,textinput 会把
//     换行折叠成空格,所以这一条是粘贴场景的必经之路);
//  2. 流解析一个都拿不到时,退回逐行解析(JSONL 文件),跳过无效行并计数。
//
// 尾部有无法解析的残留时只计数不报错:导入是"能救多少救多少"。
func parseJSONDocs(data []byte) (docs []any, badDocs int, err error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var v any
		derr := dec.Decode(&v)
		if derr != nil {
			if len(docs) > 0 {
				if !errors.Is(derr, io.EOF) {
					badDocs++ // 尾部残留
				}
				return docs, badDocs, nil
			}
			break
		}
		docs = append(docs, v)
	}

	// 流解析失败:按行重试(JSONL / 逐行拼接)。只接受对象与数组,
	// 单独的一行字符串/数字不是账号文档。
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var v any
		if json.Unmarshal([]byte(line), &v) != nil {
			badDocs++
			continue
		}
		switch v.(type) {
		case map[string]any, []any:
			docs = append(docs, v)
		default:
			badDocs++
		}
	}
	return docs, badDocs, nil
}

// infoView "m 信息"面板:官方目录与账号库状态。
func (m ChatgptModel) infoView() string {
	val := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	accentSt := lipgloss.NewStyle().Foreground(cinfo.accent)
	authPath := ""
	if m.home != "" {
		authPath = gptauth.AuthFilePath(m.home)
	}
	var current string
	var curRows []string
	if m.current == nil {
		curRows = append(curRows, styleDim.Render("(未登录)"))
	} else {
		curRows = append(curRows, accentSt.Render(prettyAuthMode(m.current.AuthMode)))
		if m.current.Email != "" {
			curRows = append(curRows, val.Render(m.current.Email))
		}
		if m.current.AccountID != "" {
			curRows = append(curRows, styleDim.Render(m.current.AccountID))
		}
		if m.current.LastRefresh != "" {
			curRows = append(curRows, styleDim.Render("最近刷新 "+refreshText(m.current.LastRefresh)))
		}
	}
	current = strings.Join(curRows, "\n")
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = "~/.codex"
	}
	codexState := styleDim.Render("未检测到")
	if gptauth.CodexRunning() {
		codexState = accentSt.Render("正在运行")
	}
	curLines := strings.Split(current, "\n")
	curBlock := styleDim.Render("当前凭据:  ") + curLines[0]
	for _, extra := range curLines[1:] {
		curBlock += "\n" + strings.Repeat(" ", 11) + extra
	}
	return strings.Join([]string{
		styleTitle.Render("信息"),
		styleDim.Render("官方目录:  ") + val.Render(orDash(m.home)),
		styleDim.Render("auth.json: ") + val.Render(orDash(authPath)),
		curBlock,
		styleDim.Render("账号库:    ") + val.Render(gptstore.DefaultPath()) +
			accentSt.Render(fmt.Sprintf("  %d 个账号", len(m.accounts))),
		styleDim.Render("Codex:     ") + codexState,
		styleDim.Render("CODEX_HOME:") + val.Render(codexHome),
		"",
		styleDim.Render("按任意键返回"),
	}, "\n")
}

func (m ChatgptModel) detailView(a *gptstore.Account) string {
	lines := []string{
		styleTitle.Render(a.Label),
		"email:         " + orDash(a.Email),
		"account_id:    " + orDash(a.AccountID),
		"plan:          " + orDash(a.PlanType),
	}
	if a.WorkspaceID != "" {
		lines = append(lines, "workspace_id:  "+a.WorkspaceID)
	}
	lines = append(lines,
		"source:        "+a.SourceFormat+"("+orDash(a.SourceName)+")",
		"imported_at:   "+a.ImportedAt.Format(time.RFC3339),
	)
	if a.Refreshable {
		lines = append(lines, "refresh:       有 refresh_token,官方端可自动续期")
	} else if a.AccessExpiresAt != nil {
		lines = append(lines, "expires:       "+a.AccessExpiresAt.Format(time.RFC3339)+"(无 refresh_token,过期后需重新导入)")
	} else {
		lines = append(lines, "expires:       未知(无 refresh_token)")
	}
	if a.IDTokenSynthetic {
		note := "占位(输入中无真实 id_token)"
		if a.IDTokenReplaced {
			note = "占位(与 account_id 不一致,已替换)"
		}
		lines = append(lines, "id_token:      "+note)
	}
	lines = append(lines,
		"access_token:  "+mask(a.Tokens.AccessToken),
		"refresh_token: "+mask(a.Tokens.RefreshToken),
	)
	if a.Tokens.SessionToken != "" {
		lines = append(lines, "session_token: "+mask(a.Tokens.SessionToken))
	}
	lines = append(lines, "", styleDim.Render("按任意键返回"))
	return strings.Join(lines, "\n")
}

func cHelpView(width int) string {
	return renderHelp("按键说明(ChatGPT 账号切换器)", []helpEntry{
		{"j/k 或 ↑/↓", "移动光标"},
		{"←/→", "在底部操作栏中选择操作,回车执行"},
		{"enter", "激活选中账号(再按一次 Enter 确认):写入 Codex 的凭据"},
		{"i", "导入会话凭据 JSON(WebSession / sub2api)"},
		{"", "输入文件路径,或直接把 JSON 粘贴进输入框;导入只入库,不自动激活"},
		{"s", "保存当前 auth.json 凭据到账号库(输入显示名)"},
		{"r", "重命名选中账号"},
		{"d", "从账号库移除(按两次 d 确认,移入回收站)"},
		{"z", "撤销最近一次删除"},
		{"n", "登出:清空本地 auth.json(账号保留在账号库),用于重新登录新账号"},
		{"m", "查看官方目录/账号库信息"},
		{"?", "本帮助"},
		{"Esc", "返回目标软件选择"},
		{"q / Q / Ctrl+C", "退出"},
	}, []string{styleDim.Render("账号库位置:" + gptstore.DefaultPath())}, width)
}

func orDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}
