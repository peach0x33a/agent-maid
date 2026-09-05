// 各工具界面共享的交互脚手架:界面模式、状态行、底部操作栏、
// 删除待确认("再按一次 d")的生命周期与摇晃动画。
// 工具模型嵌入 baseModel,只实现自己的数据与动作。
package ui

import (
	"strings"
	"time"

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

// status 底部状态行。
type status struct {
	kind int // 0 info 1 ok 2 err 3 warn 4 err-无图标(红色确认提示)
	text string
}

func (s status) render() string {
	switch s.kind {
	case 1:
		return styleOk.Render("✔ " + s.text)
	case 2:
		return styleErr.Render("✘ " + s.text)
	case 3:
		return styleWarn.Render("⚠ " + s.text)
	case 4:
		return styleErr.Render(s.text)
	case 5:
		return lipgloss.NewStyle().Reverse(true).Render(s.text)
	default:
		return styleDim.Render(s.text)
	}
}

// ───────────────────────── 底部操作栏 ─────────────────────────

// chip 底部操作栏的一个操作项(可直接按 key 触发,也可左右键选中后回车)。
type chip struct {
	key   string
	label string
}

// chipBar 渲染操作栏,sel 为选中项(高亮),accent 为平台强调色。
func chipBar(chips []chip, sel int, accent lipgloss.Color) string {
	parts := make([]string, 0, len(chips))
	for i, c := range chips {
		if i == sel {
			st := lipgloss.NewStyle().Bold(true).
				Foreground(lipgloss.Color("#15151E")).Background(accent)
			parts = append(parts, st.Render(" "+c.key+" "+c.label+" "))
		} else {
			parts = append(parts, styleDim.Render(" "+c.key+" "+c.label+" "))
		}
	}
	return strings.Join(parts, " ")
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

// cursorGlyph 列表光标符号。
func cursorGlyph() string {
	if plainMode {
		return "> "
	}
	return "❯ "
}

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
}

// quitArmDelay 第二次 Q 需距首次至少 0.5s 才生效(防误触连按)。
const quitArmDelay = 500 * time.Millisecond

// autoRelaunch 替换完成后自动打开/重启对应软件(向导偏好,默认开启)。
var autoRelaunch = true

// armSwitch 进入"再按一次 Enter 确认切换"状态(提示显示在选中行徽章位置)。
func (b *baseModel) armSwitch(name string) {
	b.switchArm = true
	b.switchName = name
}

// confirmSwitch 第二次 Enter:结束待确认态并返回是否确实处于待确认。
func (b *baseModel) confirmSwitch() bool {
	armed := b.switchArm
	b.switchArm = false
	return armed
}

// confirmBadge 切换确认提示,渲染在"使用中"徽章的位置。
func confirmBadge(info *targetInfo) string {
	return lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("#15151E")).Background(info.accent).
		Render(" 再按一次 Enter 确认切换 ")
}

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
	b.status = status{5, " 再按一次 Q 退出 "}
	return nil
}

// header 顶部横幅 + 全宽分隔线(横幅的线稍亮)。
func (b baseModel) header(info *targetInfo) string {
	return banner(info) + "\n" + b.rule("240")
}

// section 小节标题 + 全宽分隔线(比横幅的更淡一档)。
func (b baseModel) section(text string) string {
	return styleDim.Render(text) + "\n" + b.rule("238")
}

// rule 渲染铺满终端宽度的水平分隔线。
func (b baseModel) rule(color string) string {
	w := b.width
	if w <= 0 {
		w = 80
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).
		Render(strings.Repeat("─", w))
}

func (b *baseModel) setStatus(err error, okText, errPrefix string) {
	if err != nil {
		b.status = status{2, errPrefix + ": " + err.Error()}
	} else {
		b.status = status{1, okText}
	}
}

// clearPrompt 撤下红色删除提示与退出提示(切换模式/移动光标/超时/返回首屏时调用)。
func (b *baseModel) clearPrompt() {
	b.deleteArm = false
	b.quitArm = false
	b.switchArm = false
	if b.status.kind == 4 || b.status.kind == 5 {
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
	b.status = status{4, text}
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
		b.shakeFrame++
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

// renderStatus 渲染状态行;红色确认提示附加摇晃缩进。
func (b baseModel) renderStatus() string {
	if b.status.kind == 4 {
		return status{4, shakePrefix(b.shakeFrame) + b.status.text}.render()
	}
	return b.status.render()
}

// bottomBar 底部操作栏与提示行(附仓库地址)。
func (b baseModel) bottomBar(info *targetInfo, chips []chip) string {
	bar := chipBar(chips, b.chipIdx, info.accent)
	hint := styleDim.Render("↑/↓ 选条目 · ←/→ 选操作 · 回车执行选中操作 · Esc 返回选择 · q/Q 退出 · " + meta.Repo)
	return bar + "\n" + hint
}

// confirmBox 确认框:警示文案 + 摇晃的按键行。
func (b baseModel) confirmBox(text, keys string) string {
	return "\n" + styleBox.Render(styleWarn.Render(text)+"\n"+shakePrefix(b.shakeFrame)+keys) + "\n"
}
