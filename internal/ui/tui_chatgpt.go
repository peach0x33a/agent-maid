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

	"agent-maid/internal/gptauth"
	"agent-maid/internal/gptsession"
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
	{"s", "[S]ave 保存"},
	{"i", "[I]mport 导入"},
	{"r", "[R]ename 重命名"},
	{"d", "[D]elete 删除"},
	{"n", "[N]ew 新建"},
	{"m", "[M]eta 信息"},
	{"?", "[?]Help 帮助"},
}

type ChatgptModel struct {
	baseModel

	loadErr    error
	positioned bool // 首次加载数据后已把光标定位到使用中的账号

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
		m.width = msg.Width
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
		if _, ok := msg.(tea.KeyMsg); ok {
			m.mode = modeNormal
		}
		return m, nil
	}
	return m.updateNormal(msg)
}

func (m ChatgptModel) updateNormal(msg tea.Msg) (Manager, tea.Cmd) {
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
			err := m.doRemove(sel.ID)
			m.setStatus(err, "已从账号库移除 "+sel.Label+"(官方 auth.json 未改动)", "移除失败")
			return m, func() tea.Msg { return cRefreshMsg{} }
		}
		cmd := m.armPrompt(sel.ID, fmt.Sprintf("再按一次 d 删除账号 %q", sel.Label))
		return m, cmd
	case "n", "N": // 登出:清空本地 auth.json(账号保留在账号库)
		if m.current == nil {
			m.status = status{2, "当前未登录,无需登出"}
			return m, nil
		}
		if m.deleteArm && m.deleteName == "N" {
			m.clearPrompt()
			if _, err := m.saveCurrent(""); err != nil {
				m.status = status{2, "保存当前凭据失败: " + err.Error()}
				return m, nil
			}
			if err := gptauth.ClearAuthFile(m.home); err != nil {
				m.status = status{2, "清空凭据失败: " + err.Error()}
				return m, nil
			}
			m.status = status{1, "已登出 Codex(账号已保留在账号库),重新登录后按 s 保存"}
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
		m.mode = modeMachine
	case "?", "h":
		m.clearPrompt()
		m.mode = modeHelp
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
	home, err := gptauth.ResolveHome("")
	if err != nil {
		return "", err
	}
	tokens, authMode, found, err := gptauth.ReadCurrentTokens(home)
	if err != nil {
		return "", err
	}
	if !found {
		if authMode != "" && authMode != gptauth.AuthModeChatGPT {
			return "", fmt.Errorf("当前是 %s 认证,没有可保存的 ChatGPT 凭据", prettyAuthMode(authMode))
		}
		return "", fmt.Errorf("官方目录没有可保存的凭据(%s)", gptauth.AuthFilePath(home))
	}
	idPayload := gptsession.JWTPayload(tokens.IDToken)
	accessPayload := gptsession.JWTPayload(tokens.AccessToken)
	email, _ := idPayload["email"].(string)
	authClaim := gptsession.AuthClaimOf(accessPayload)
	if authClaim == nil {
		authClaim = gptsession.AuthClaimOf(idPayload)
	}
	planType, _ := authClaim["chatgpt_plan_type"].(string)
	userID, _ := authClaim["chatgpt_user_id"].(string)

	if label == "" {
		label = firstNonEmptyStr(email, tokens.AccountID, "auth.json")
	}
	account := gptsession.Account{
		Label:        label,
		Email:        email,
		AccountID:    tokens.AccountID,
		UserID:       userID,
		PlanType:     planType,
		SourceFormat: "auth.json",
		SourceName:   filepath.Base(home),
		Refreshable:  tokens.RefreshToken != "",
		Tokens: gptsession.Tokens{
			IDToken:      tokens.IDToken,
			AccessToken:  tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
		},
	}
	if !account.Refreshable && accessPayload != nil {
		if exp, ok := accessPayload["exp"].(float64); ok && exp > 0 {
			t := time.Unix(int64(exp), 0).UTC()
			account.AccessExpiresAt = &t
		}
	}
	st, err := gptstore.Load(gptstore.DefaultPath())
	if err != nil {
		return "", err
	}
	stored := gptstore.FromSession(account, time.Now().UTC())
	_, idx := st.Find(stored.ID)
	existed := idx >= 0
	st.Upsert(stored)
	if st.Data.ActiveID == "" {
		st.Data.ActiveID = stored.ID
	}
	if err := st.Save(); err != nil {
		return "", err
	}
	verb := "已保存当前凭据到账号库"
	if existed {
		verb = "凭据已在账号库中,已更新"
	}
	return fmt.Sprintf("%s:%s", verb, stored.Label), nil
}

// renameAccount 改账号库里的显示名。
func (m *ChatgptModel) renameAccount(id, label string) error {
	st, err := gptstore.Load(gptstore.DefaultPath())
	if err != nil {
		return err
	}
	acc, _ := st.Find(id)
	if acc == nil {
		return fmt.Errorf("在账号库中找不到 %q", id)
	}
	acc.Label = label
	return st.Save()
}

// importAccounts 解析输入(文件路径或粘贴的 JSON),转换后入库。不自动激活。
func (m *ChatgptModel) importAccounts(raw string) status {
	var data []byte
	src := "pasted-json"
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		data = []byte(raw)
	} else {
		b, err := os.ReadFile(expandHome(raw))
		if err != nil {
			return status{2, "读取文件失败: " + err.Error()}
		}
		data = b
		src = filepath.Base(expandHome(raw))
	}
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	if len(data) == 0 {
		return status{2, "输入为空"}
	}
	docs, badDocs, perr := parseJSONDocs(data)
	if perr != nil {
		return status{2, "输入不是合法 JSON: " + perr.Error()}
	}
	// 多段文档(JSONL/拼接)时,来源名附带序号便于追溯
	var records []gptsession.Record
	for i, doc := range docs {
		name := src
		if len(docs) > 1 {
			name = fmt.Sprintf("%s#%d", src, i+1)
		}
		records = append(records, gptsession.Collect(doc, name)...)
	}
	if len(records) == 0 {
		msg := "未在输入中找到包含 access_token 的会话/账号对象(支持 WebSession 与 sub2api 导出)"
		if badDocs > 0 {
			msg += fmt.Sprintf("(另有 %d 个无效文档)", badDocs)
		}
		return status{2, msg}
	}
	st, err := gptstore.Load(gptstore.DefaultPath())
	if err != nil {
		return status{2, "读取账号库失败: " + err.Error()}
	}
	now := time.Now().UTC()
	imported := 0
	skipped := 0
	first := ""
	for _, rec := range records {
		converted, err := gptsession.Convert(rec, now)
		if err != nil {
			skipped++
			continue
		}
		account := gptstore.FromSession(converted, now)
		st.Upsert(account)
		if first == "" {
			first = account.Label
		}
		imported++
	}
	if imported == 0 {
		msg := fmt.Sprintf("全部 %d 项均无法转换", len(records))
		if badDocs > 0 {
			msg += fmt.Sprintf("(另有 %d 个无效文档)", badDocs)
		}
		return status{2, msg}
	}
	if err := st.Save(); err != nil {
		return status{2, "保存账号库失败: " + err.Error()}
	}
	tail := ""
	if skipped > 0 {
		tail += fmt.Sprintf(",跳过 %d 项", skipped)
	}
	if badDocs > 0 {
		tail += fmt.Sprintf(",忽略 %d 个无效文档", badDocs)
	}
	if imported == 1 {
		return status{1, fmt.Sprintf("已导入 %s%s,Enter 激活并写入官方目录", first, tail)}
	}
	return status{1, fmt.Sprintf("已导入 %d 个账号%s,选中后按 Enter 激活", imported, tail)}
}

// doActivate 把账号投影为官方 auth.json(保序合并、原子写入)。
func (m *ChatgptModel) doActivate(id string) (string, error) {
	st, err := gptstore.Load(gptstore.DefaultPath())
	if err != nil {
		return "", err
	}
	acc, _ := st.Find(id)
	if acc == nil {
		return "", fmt.Errorf("在账号库中找不到 %q", id)
	}
	st.Data.ActiveID = acc.ID
	if err := st.Save(); err != nil {
		return "", err
	}
	home, err := gptauth.ResolveHome("")
	if err != nil {
		return "", err
	}
	existing, err := gptauth.ReadAuthFile(home)
	if err != nil {
		return "", err
	}
	merged, err := gptauth.MergeAuthFile(existing, gptauth.Tokens{
		IDToken:      acc.Tokens.IDToken,
		AccessToken:  acc.Tokens.AccessToken,
		RefreshToken: acc.Tokens.RefreshToken,
		AccountID:    acc.AccountID,
	}, time.Now().UTC())
	if err != nil {
		return "", err
	}
	if _, err := gptauth.WriteAuthFile(home, merged); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	acc.LastUsedAt = &now
	_ = st.Save()
	return "已激活 " + acc.Label + ",已写入 Codex 凭据(" + gptauth.AuthFilePath(home) + ")", nil
}

// doRemove 从账号库移除,不动官方 auth.json。
func (m *ChatgptModel) doRemove(id string) error {
	st, err := gptstore.Load(gptstore.DefaultPath())
	if err != nil {
		return err
	}
	if _, ok := st.Remove(id); !ok {
		return fmt.Errorf("在账号库中找不到 %q", id)
	}
	return st.Save()
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

// accountTable 渲染账号列表表格(表头 + 动态列宽)。// accountTable 渲染账号列表表格(表头 + 动态列宽)。
func (m ChatgptModel) accountTable() string {
	rows := [][]string{{"  账号", "套餐", "来源", "ID", "最后使用", "状态"}}
	for i, a := range m.accounts {
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
	return joinTable(rows)
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
		overlay = styleBox.Render(purpose + "\n" + m.input.View())
	case modeDetail:
		if sel := m.selected(); sel != nil {
			overlay = styleBox.Render(m.detailView(sel))
		}
	case modeHelp:
		overlay = styleBox.Render(cHelpView())
	case modeMachine:
		overlay = styleBox.Render(m.infoView())
	}

	footer := m.bottomBar(&cinfo, cChips)
	page := m.header(&cinfo) + "\n" + centerBlock(body.String(), w) + "\n"
	if overlay != "" {
		page += m.centerBox(overlay) + "\n"
	}
	return page + centerBlock(footer, w)
}

func (m ChatgptModel) findByID(id string) *gptstore.Account {
	for _, a := range m.accounts {
		if a.ID == id {
			return a
		}
	}
	return nil
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
//  1. 连续多段 JSON —— 单个对象/数组、CPA、sub2api 整包、拼接 JSON 均可;
//  2. JSONL(每行一个 JSON 对象,常见于批量导出);
//  3. 上述失败时退回逐行解析,跳过无效行并计数。
func parseJSONDocs(data []byte) (docs []any, badDocs int, err error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var v any
		if derr := dec.Decode(&v); derr != nil {
			if errors.Is(derr, io.EOF) {
				return docs, 0, nil
			}
			break
		}
		docs = append(docs, v)
	}
	docs = nil
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var v any
		if json.Unmarshal([]byte(line), &v) == nil {
			docs = append(docs, v)
		} else {
			badDocs++
		}
	}
	if len(docs) == 0 {
		return nil, badDocs, errors.New("未能解析出任何 JSON 文档")
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

func cHelpView() string {
	return strings.Join([]string{
		styleTitle.Render("按键说明(ChatGPT 账号切换器)"),
		"j/k 或 ↑/↓   移动光标",
		"←/→          在底部操作栏中选择操作,回车执行",
		"enter        激活选中账号(再按一次 Enter 确认):写入 Codex 的凭据,原文件自动备份",
		"i            导入会话凭据 JSON(WebSession / sub2api),",
		styleDim.Render("             输入文件路径,或直接把 JSON 粘贴进输入框;导入只入库,不自动激活"),
		"s            保存当前 auth.json 凭据到账号库(输入显示名)",
		"S            快存当前凭据(自动命名)",
		"r            重命名选中账号",
		"d            从账号库移除(按两次 d 确认,不动官方 auth.json)",
		"m            查看官方目录/账号库信息",
		"?            本帮助",
		"Esc          返回目标软件选择",
		"q / Q / Ctrl+C 退出",
		"",
		styleDim.Render("账号库位置:" + gptstore.DefaultPath()),
		"",
		styleDim.Render("按任意键返回"),
	}, "\n")
}

func orDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

func shortID(id string) string {
	if id == "" {
		return "—"
	}
	if len(id) <= 12 {
		return id
	}
	return id[:12] + "…"
}
