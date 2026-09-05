package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func splitLines(s string) []string { return strings.Split(s, "\n") }

func joinLines(lines []string) string { return strings.Join(lines, "\n") }

// Target 表示用户要管理的目标软件。
type Target int

const (
	TargetNone Target = iota
	TargetChatGPT
	TargetZCode
)

// 图标像素矩阵来自官方图标(ChatGPT = OpenAI 官方花形矢量;ZCode = 桌面端
// 安装包内 512×512 图标),由 gen 脚本栅格化为 ink 等级(见 logos_art.go)。
// 渲染为半块方块并沿对角线做品牌渐变着色。

var gradDim = []string{"#3F3F46", "#8A8A98", "#3F3F46"} // 未选中:灰阶渐变

type targetInfo struct {
	name    string
	desc    string
	path    string
	pxLarge []string
	pxSmall []string
	grad    []string       // 品牌渐变(对角)
	accent  lipgloss.Color // 选中时名称颜色
}

var targets = []targetInfo{
	{
		name:    "ChatGPT",
		desc:    "Codex 登录账号切换",
		path:    "~/.codex/auth.json",
		pxLarge: chatgptPxLarge,
		pxSmall: chatgptPxSmall,
		grad:    []string{"#5EEAD4", "#10A37F", "#0EA5E9"},
		accent:  lipgloss.Color("#10A37F"),
	},
	{
		name:    "ZCode",
		desc:    "桌面端账号槽位切换",
		path:    "~/.zcode/v2/credentials.json",
		pxLarge: zcodePxLarge,
		pxSmall: zcodePxSmall,
		grad:    []string{"#F472B6", "#A855F7", "#6366F1"},
		accent:  lipgloss.Color("212"),
	},
}

// renderArt 把图标渲染为渐变半块方块(多行字符串)。
func (t targetInfo) renderArt(small bool, dim bool) string {
	px := t.pxLarge
	if small {
		px = t.pxSmall
	}
	stops := t.grad
	if dim {
		stops = gradDim
	}
	return renderHalfblock(px, stops)
}
