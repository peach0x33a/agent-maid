// 各工具界面共享的交互脚手架:界面模式、状态行、底部操作栏、
// 删除待确认("再按一次 d")的生命周期与摇晃动画。
// 工具模型嵌入 baseModel,只实现自己的数据与动作。
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"agent-maid/internal/meta"
)

// ───────────────────────── 模式/输入用途 ─────────────────────────

// mode 两个目标管理器共用的界面模式。
type mode int

const (
	modeNormal  mode = iota
	modeInput        // 输入框(ZCode: 保存/重命名;ChatGPT: 导入/保存/重命名)
	modeConfirm      // 确认操作
	modeWaiting      // ZCode 专用:等待 ZCode 退出后执行切换
	modeHelp
	modeMachine // 机器/环境信息面板
	modeDetail  // ChatGPT 专用:账号详情
)

// inputPurpose 输入框用途。
type inputPurpose int

const (
	inputSave    inputPurpose = iota // ZCode:保存当前账号
	inputRename                      // ZCode:重命名槽位
	inputImport                      // ChatGPT:导入 JSON 文件路径或粘贴内容
	inputGSave                       // ChatGPT:保存当前凭据(输入显示名)
	inputGRename                     // ChatGPT:重命名账号显示名
)

// ───────────────────────── 状态行 ─────────────────────────

// 状态行种类。
const (
	statusInfo  = 0 // 普通提示
	statusOK    = 1 // 成功
	statusErr   = 2 // 失败
	statusWarn  = 3 // 警告
	statusArm   = 4 // 待确认提示(红色,带摇晃):删除/登出
	statusQuit  = 5 // 反选高亮:再按一次 Q 退出
	statusEnter = 6 // 待确认切换(强调色):再按一次 Enter
)

// status 底部状态行。
type status struct {
	kind int // 见上方 statusInfo..statusEnter
	text string
}

func (s status) render() string {
	switch s.kind {
	case statusOK:
		return styleOk.Render("✔ " + s.text)
	case statusErr:
		return styleErr.Render("✘ " + s.text)
	case statusWarn:
		return styleWarn.Render("⚠ " + s.text)
	case statusArm:
		return styleErr.Render(s.text)
	case statusQuit:
		return lipgloss.NewStyle().Reverse(true).Render(s.text)
	case statusEnter:
		return styleSel.Render(s.text)
	default:
		return styleDim.Render(s.text)
	}
}

// ───────────────────────── 底部操作栏 ─────────────────────────

// chip 底部操作栏的一个操作项(可直接按 key 触发,也可左右键选中后回车)。
// disp 为展示文本,形如 "[S]ave 保存"(键字母在方括号内高亮)。
type chip struct {
	key  string
	disp string
}

// chipOne 渲染单个操作项(选中项用强调色底)。
func chipOne(c chip, selected bool, accent lipgloss.Color) string {
	if selected {
		st := lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color("#15151E")).Background(accent)
		return st.Render(" " + c.disp + " ")
	}
	if idx := strings.Index(c.disp, "]"); idx > 0 {
		keyPart := lipgloss.NewStyle().Bold(true).Foreground(accent).Render(c.disp[:idx+1])
		return keyPart + styleDim.Render(c.disp[idx+1:])
	}
	return styleDim.Render(" " + c.disp + " ")
}

// chipBar 渲染操作栏,sel 为选中项(高亮),accent 为平台强调色。
func chipBar(chips []chip, sel int, accent lipgloss.Color) string {
	parts := make([]string, 0, len(chips))
	for i, c := range chips {
		parts = append(parts, chipOne(c, i == sel, accent))
	}
	return strings.Join(parts, "  ")
}

// chipBarLines 按终端宽度把操作栏折成多行:终端的行尾超宽会被裁剪,
// 直接一行铺开会让 80/100 列下后半截操作([N]/[M]/[?])完全看不见。
func chipBarLines(chips []chip, sel int, accent lipgloss.Color, width int) []string {
	if width <= 0 {
		width = 80
	}
	gap := "  "
	avail := width - 1
	var lines []string
	var cur []string
	curW := 0
	for i, c := range chips {
		one := chipOne(c, i == sel, accent)
		oneW := lipgloss.Width(one)
		addW := oneW
		if len(cur) > 0 {
			addW += lipgloss.Width(gap)
		}
		if len(cur) > 0 && curW+addW > avail {
			lines = append(lines, strings.Join(cur, gap))
			cur = nil
			curW = 0
			addW = oneW
		}
		cur = append(cur, one)
		curW += addW
	}
	if len(cur) > 0 {
		lines = append(lines, strings.Join(cur, gap))
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// ───────────────────────── 摇晃动画节拍 ─────────────────────────

// statusTickMsg 提示/确认框摇晃动画与提示超时的节拍。
type statusTickMsg struct{}

// shakeOffsets 水平摇晃偏移序列:衰减钟摆,摇完一轮回到原位后静止。
var shakeOffsets = [8]int{3, 4, 3, 2, 1, 2, 1, 0}

func statusTick() tea.Cmd {
	return tea.Tick(110*time.Millisecond, func(time.Time) tea.Msg { return statusTickMsg{} })
}

// shakePrefix 按帧序号返回摇晃缩进;序列播完即静止(不再持续跳动)。
func shakePrefix(frame int) string {
	if frame >= len(shakeOffsets) {
		return ""
	}
	return strings.Repeat(" ", shakeOffsets[frame])
}

// banner 子界面顶部持续显示的标题行(分隔线见 header)。
func banner(info *targetInfo) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).
		Render("AgentMaid - " + info.Name + "账号管理")
}

// ───────────────────────── 纯文本降级(向导判定符号不支持时) ─────────────────────────

// plainMode 由设置向导写入:终端无法显示特殊符号时,图标/光标/徽章退化为纯文本。
var plainMode bool

// noAnim 由设置写入:关闭流光/摇晃/转轮等动画。
var noAnim bool

// badgeInUse "使用中"徽章:常规为绿底深字;纯文本降级为绿色方括号文本。
func badgeInUse() string {
	if plainMode {
		return styleOk.Render(" [使用中] ")
	}
	return lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("#15151E")).Background(lipgloss.Color("42")).
		Render(" 使用中 ")
}

// ───────────────────────── 共享模型状态 ─────────────────────────

// baseModel 各工具界面共享的交互状态与提示生命周期。
type baseModel struct {
	width      int // 终端宽度(WindowSizeMsg 时写入,用于全宽分隔线)
	height     int // 终端高度(列表滚动窗口用)
	mode       mode
	quit       bool // 已请求退出
	status     status
	chipIdx    int       // 操作栏选中项
	shakeFrame int       // 摇晃动画帧序号
	deleteArm  bool      // 已按过一次 d,等待再次确认
	deleteName string    // 待删除项的标识(槽位名/账号 ID)
	deleteAt   time.Time // 进入待确认的时刻(超时自动撤下)
	switchArm  bool      // 已按过一次 enter,等待再次确认切换
	switchName string    // 待切换项的标识
	quitArm    bool      // 已按过一次 q,等待再次确认退出
	quitAt     time.Time // 首次按下 q 的时刻(0.5s 内的第二次按下无效)
	panelTop   int       // 帮助/信息面板的滚动位置(行)
}

// quitArmDelay 第二次 Q 需距首次至少 0.5s 才生效(防误触连按)。
const quitArmDelay = 500 * time.Millisecond

// armSwitch 进入"再按一次 Enter 确认切换"状态:状态行给出待确认提示,
// 任意其他键都会取消(clearPrompt)。
func (b *baseModel) armSwitch(name string) {
	b.switchArm = true
	b.switchName = name
	b.deleteArm = false
	b.quitArm = false
	b.status = status{statusEnter, "再按一次 Enter 确认切换到 " + name + "(按其他键取消)"}
}

// confirmSwitch 第二次 Enter:结束待确认态并返回是否确实处于待确认。
func (b *baseModel) confirmSwitch() bool {
	armed := b.switchArm
	b.switchArm = false
	if b.status.kind == statusEnter {
		b.status = status{}
	}
	return armed
}

// isReloadKey 手动刷新键(ctrl+r):软件在 TUI 开着时登录/退出登录/在别处
// 切换了账号之后,让界面重新读盘,不必退回首屏再进来。
func isReloadKey(key string) bool { return key == "ctrl+r" }

// handleQuitKey 处理 q/Q:首次按下给出反选高亮提示;第二次按下须间隔
// 0.5s 以上才真正退出;期间按其他键自动取消。
func (b *baseModel) handleQuitKey() tea.Cmd {
	if b.quitArm {
		if time.Since(b.quitAt) >= quitArmDelay {
			b.quit = true
			return tea.Quit
		}
		return nil // 间隔不足,忽略本次按下
	}
	b.deleteArm = false
	b.quitArm = true
	b.quitAt = time.Now()
	b.status = status{statusQuit, " 再按一次 Q 退出 "}
	return nil
}

// header 顶部横幅 + 全宽分隔线(横幅的线稍亮)。
func (b baseModel) header(info *targetInfo) string {
	return banner(info) + "\n" + b.rule("240")
}

// rule 渲染铺满终端宽度的水平分隔线(横幅用)。
func (b baseModel) rule(color string) string {
	return ruleW(func() int {
		if b.width > 0 {
			return b.width
		}
		return 80
	}(), color)
}

// ruleW 渲染指定宽度的水平分隔线(正文块内使用)。
func ruleW(w int, color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).
		Render(strings.Repeat("─", w))
}

func (b *baseModel) setStatus(err error, okText, errPrefix string) {
	if err != nil {
		b.status = status{statusErr, errPrefix + ": " + err.Error()}
	} else {
		b.status = status{statusOK, okText}
	}
}

// clearPrompt 撤下红色删除提示与退出提示(切换模式/移动光标/超时/返回首屏时调用)。
func (b *baseModel) clearPrompt() {
	b.deleteArm = false
	b.quitArm = false
	b.switchArm = false
	if b.status.kind == statusArm || b.status.kind == statusQuit || b.status.kind == statusEnter {
		b.status = status{}
	}
}

// armPrompt 进入"再按一次 d"待确认状态。name 为待删项标识(再次按 d 时
// 与当前选中项比对),text 为完整提示文案。返回动画节拍(首次进入时)。
func (b *baseModel) armPrompt(name, text string) tea.Cmd {
	wasArmed := b.deleteArm
	b.deleteArm = true
	b.deleteName = name
	b.deleteAt = time.Now()
	b.quitArm = false
	b.shakeFrame = 0 // 重新武装时重新摇晃
	b.status = status{statusArm, text}
	if !wasArmed {
		return statusTick()
	}
	return nil
}

// handleStatusTick 处理提示节拍:超时自动撤下、推进摇晃动画。
// 返回后续命令(nil 表示动画结束)。
func (b *baseModel) handleStatusTick() tea.Cmd {
	if b.deleteArm && time.Since(b.deleteAt) >= promptTimeout {
		b.clearPrompt()
		return nil
	}
	if b.deleteArm || b.mode == modeConfirm {
		if !noAnim {
			b.shakeFrame++
		}
		return statusTick()
	}
	return nil
}

// chipNav 左右移动操作栏选中项(dir 为 ±1,count 为操作项总数)。
func (b *baseModel) chipNav(dir, count int) {
	if count == 0 {
		return
	}
	b.chipIdx = ((b.chipIdx+dir)%count + count) % count
}

// listChromeRows 列表以外的固定行数(横幅/分隔线/当前账号/说明/状态/操作栏
// /提示行/各段空行)。用于给列表留出滚动窗口,避免内容超屏把标题与操作栏
// 顶出可视区。
const listChromeRows = 13

// listRows 当前列表可占用的行数(至少 3 行:表头 + 2 条数据)。
func (b baseModel) listRows() int {
	h := b.height
	if h <= 0 {
		h = 24
	}
	rows := h - listChromeRows
	if b.mode == modeWaiting || b.mode == modeInput || b.mode == modeConfirm ||
		b.mode == modeHelp || b.mode == modeMachine || b.mode == modeDetail {
		rows -= 6 // 浮层还要占地方
	}
	if rows < 3 {
		rows = 3
	}
	return rows
}

// windowRange 计算列表可见区间 [start,end):保证 cursor 在窗口内,
// total 小于窗口时全量显示;发生截断时额外留一行给"还有 N 个"提示。
func (b baseModel) windowRange(total, cursor int) (start, end int) {
	rows := b.listRows()
	dataRows := rows - 1 // 预留表头
	if total <= dataRows {
		return 0, total
	}
	dataRows-- // 预留"上方/下方还有 N 个"提示行
	if dataRows < 1 {
		dataRows = 1
	}
	start = 0
	if cursor >= dataRows {
		start = cursor - dataRows + 1
	}
	if maxStart := total - dataRows; start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	return start, start + dataRows
}

// renderStatus 渲染状态行;红色确认提示附加摇晃缩进。
func (b baseModel) renderStatus() string {
	if b.status.kind == 4 {
		return status{statusArm, shakePrefix(b.shakeFrame) + b.status.text}.render()
	}
	return b.status.render()
}

// bottomBar 底部操作栏与提示行(附仓库地址)。操作栏按终端宽度折行,
// 避免窄终端下后半截操作被裁剪。
func (b baseModel) bottomBar(info *targetInfo, chips []chip) string {
	bar := strings.Join(chipBarLines(chips, b.chipIdx, info.accent, b.width), "\n")
	hint := wrapToWidth("↑/↓ 选条目 · ←/→ 选操作 · 回车执行选中操作 · Ctrl+R 刷新 · Esc 返回选择 · "+meta.Repo, b.width)
	return bar + "\n" + styleDim.Render(hint)
}

// wrapToWidth 按显示宽度折行,保留 ANSI 样式。先用 Wordwrap 在空格/
// 标点处断行,再对仍然超宽的行(路径、URL 这类不可断词)硬折。
func wrapToWidth(s string, limit int) string {
	if limit < 8 {
		limit = 8
	}
	wrapped := ansi.Wordwrap(s, limit, "")
	lines := strings.Split(wrapped, "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > limit {
			lines[i] = ansi.Hardwrap(l, limit, true)
		}
	}
	return strings.Join(lines, "\n")
}

// helpEntry 帮助面板的一行:按键列 + 说明列。
type helpEntry struct{ key, text string }

// renderHelp 渲染按键说明:按键列定宽,说明列按可用宽度折行并保持悬挂
// 缩进(续行对齐说明列),避免窄终端下折行后丢掉层次。
func renderHelp(title string, entries []helpEntry, footer []string, width int) string {
	keyW := 0
	for _, e := range entries {
		if w := lipgloss.Width(e.key); w > keyW {
			keyW = w
		}
	}
	keyW += 2 // 列间距
	descW := width - keyW - 4
	if descW < 16 {
		descW = 16
	}
	pad := strings.Repeat(" ", keyW)
	// styleTitle 自带 MarginBottom(1),不再额外插空行
	lines := []string{styleTitle.Render(title)}
	for _, e := range entries {
		if e.key == "" { // 说明行(无按键)
			wrapped := strings.Split(wrapToWidth(e.text, width-4), "\n")
			for _, l := range wrapped {
				lines = append(lines, pad+styleDim.Render(l))
			}
			continue
		}
		wrapped := strings.Split(wrapToWidth(e.text, descW), "\n")
		lines = append(lines, lipgloss.NewStyle().Width(keyW).Render(e.key)+wrapped[0])
		for _, l := range wrapped[1:] {
			lines = append(lines, pad+l)
		}
	}
	if len(footer) > 0 {
		lines = append(lines, "")
		lines = append(lines, footer...)
	}
	lines = append(lines, "", styleDim.Render("按任意键返回"))
	return strings.Join(lines, "\n")
}

// helpWidth 帮助面板排版宽度(浮层外框与内边距占 8 列)。
func (b baseModel) helpWidth() int {
	w := b.width
	if w <= 0 {
		w = 80
	}
	iw := w - 8
	if iw < 32 {
		iw = 32
	}
	return iw
}

// bottomBarLines 底栏当前占用的行数(操作栏折行 + 提示行折行)。
// 强调色只影响 ANSI 转义,不影响显示宽度,所以这里用占位色即可。
func (b baseModel) bottomBarLines(chips []chip) int {
	w := b.width
	if w <= 0 {
		w = 80
	}
	n := len(chipBarLines(chips, b.chipIdx, lipgloss.Color("212"), w))
	hint := "↑/↓ 选条目 · ←/→ 选操作 · 回车执行选中操作 · Ctrl+R 刷新 · Esc 返回选择 · " + meta.Repo
	return n + len(strings.Split(wrapToWidth(hint, w), "\n"))
}

// panelHeight 浮层可用的总行数:终端高度减去横幅、两侧空行与底栏。
func (b baseModel) panelHeight(chips []chip) int {
	h := b.height
	if h <= 0 {
		h = 24
	}
	rows := h - 2 - 2 - b.bottomBarLines(chips) // header 2 行 + 上下各 1 空行 + 底栏
	if rows < 7 {
		rows = 7
	}
	return rows
}

// panelLines 浮层正文按宽度折行后的行(外框与内边距留 8 列)。
func (b baseModel) panelLines(box string) []string {
	w := b.width
	if w <= 0 {
		w = 80
	}
	maxW := w - 8
	if maxW < 24 {
		maxW = 24
	}
	return strings.Split(wrapToWidth(box, maxW), "\n")
}

// panelVisibleRows 浮层可见正文行数(外框 2 行 + 内边距 2 行)。
func (b baseModel) panelVisibleRows(chips []chip) int {
	rows := b.panelHeight(chips) - 4
	if rows < 3 {
		rows = 3
	}
	return rows
}

// renderPanel 把浮层正文排版:按宽度折行、按高度开窗(可用 ↑/↓ 滚动)、
// 统一加圆角外框。整页不超出可视区(超出会让终端整体滚动,画面错位)。
// 需要滚动时,滚动提示占用内边距那一行,不挤占正文。
func (b baseModel) renderPanel(box string, chips []chip) string {
	if strings.TrimSpace(box) == "" {
		return ""
	}
	lines := b.panelLines(box)
	total := len(lines)
	limit := b.panelVisibleRows(chips)
	top := b.panelTop
	maxTop := total - limit
	if maxTop < 0 {
		maxTop = 0
	}
	if top > maxTop {
		top = maxTop
	}
	if top < 0 {
		top = 0
	}
	hint := ""
	if total > limit {
		lines = append([]string{}, lines[top:top+limit]...)
		hint = b.scrollHint(top, limit, total)
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2)
	if hint == "" {
		return style.Render(strings.Join(lines, "\n"))
	}
	// 底边距换成滚动提示行(同样 1 行高,总高度不变)
	return style.Padding(1, 2, 0, 2).
		Render(strings.Join(lines, "\n") + "\n" + hint)
}

// scrollHint 浮层窗口底部的滚动提示。
func (b baseModel) scrollHint(top, limit, total int) string {
	end := top + limit
	switch {
	case top == 0:
		return styleDim.Render(fmt.Sprintf("──────── ↓ 还有 %d 行,↑/↓ 滚动 ────────", total-end))
	case end >= total:
		return styleDim.Render(fmt.Sprintf("──────── ↑ 上方还有 %d 行,↑/↓ 滚动 ────────", top))
	default:
		return styleDim.Render(fmt.Sprintf("──────── ↑%d 行 / ↓%d 行,↑/↓ 滚动 ────────", top, total-end))
	}
}

// panelOverflow 浮层内容在当前终端高度下是否放不下(放不下才提示可滚动)。
func (b baseModel) panelOverflow(box string, chips []chip) bool {
	return len(b.panelLines(box)) > b.panelVisibleRows(chips)
}

// scrollPanel 处理帮助/信息浮层里的 ↑/↓:返回 true 表示本次按键已被面板
// 消化(不应关闭面板)。
func (b *baseModel) scrollPanel(key, box string, chips []chip) bool {
	limit := b.panelVisibleRows(chips)
	maxTop := len(b.panelLines(box)) - limit
	if maxTop < 0 {
		maxTop = 0
	}
	switch key {
	case "up", "k":
		if b.panelTop > 0 {
			b.panelTop--
		}
		return true
	case "down", "j":
		if b.panelTop < maxTop {
			b.panelTop++
		}
		return true
	case "pgup", "g":
		b.panelTop = 0
		return true
	case "pgdown", "G":
		b.panelTop = maxTop
		return true
	}
	return false
}

// openPanel 打开浮层前重置滚动位置。
func (b *baseModel) openPanel(m mode) {
	b.panelTop = 0
	b.mode = m
}

// 浮层存在时用浮层替换正文,而不是追加在正文之后——否则高浮层会把
// 横幅和列表顶出可视区,终端整体滚动,返回后画面仍是错位的。
func (b baseModel) page(info *targetInfo, body, overlay string, chips []chip) string {
	w := b.width
	if w <= 0 {
		w = 80
	}
	content := wrapToWidth(body, w)
	if overlay != "" {
		content = b.renderPanel(overlay, chips)
	}
	return b.header(info) + "\n" + centerBlock(content, w) + "\n" + centerBlock(b.bottomBar(info, chips), w)
}

// centerBlock 整块水平居中:所有行加统一左填充,保持块内相对对齐。
// (lipgloss.Place/Align 的 Center 是逐行居中,会把宽度不同的行各自偏移)
func centerBlock(block string, w int) string {
	lines := strings.Split(block, "\n")
	maxW := 0
	for _, l := range lines {
		if lw := lipgloss.Width(l); lw > maxW {
			maxW = lw
		}
	}
	padW := (w - maxW) / 2
	if padW < 0 {
		padW = 0
	}
	pad := strings.Repeat(" ", padW)
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

// centerScreen 整块水平垂直居中(整屏页面用)。
func centerScreen(block string, w, h int) string {
	body := centerBlock(block, w)
	lines := strings.Split(body, "\n")
	padH := (h - len(lines)) / 2
	if padH > 0 {
		head := make([]string, padH)
		lines = append(head, lines...)
	}
	return strings.Join(lines, "\n")
}
