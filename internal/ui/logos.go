package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func splitLines(s string) []string { return strings.Split(s, "\n") }

func joinLines(lines []string) string { return strings.Join(lines, "\n") }

// animFrames 流光动画帧数(首尾相接循环)。
const animFrames = 24

// 图标像素矩阵来自官方图标(ChatGPT = OpenAI 官方花形矢量;ZCode = 桌面端
// 安装包内 512×512 图标),由 gen 脚本栅格化为 ink 等级(见 logos_art.go)。
// 渲染为半块方块并沿对角线做品牌渐变着色。

// gradDim 未选中时的灰阶渐变。
var gradDim = []string{"#3F3F46", "#8A8A98", "#3F3F46"}

// targetInfo 一个目标软件的展示信息。各工具在各自的 TUI 文件中定义,
// 经 Manager.Info() 暴露给首屏与横幅。
type targetInfo struct {
	Key     string // CLI 预选名(如 zcode / chatgpt)
	Name    string // 展示名(横幅为 "AgentMaid - <Name>账号管理")
	Desc    string // 一句话说明
	Path    string // 凭据落点(展示用)
	pxLarge []string
	pxSmall []string
	grad    []string       // 品牌渐变(对角)
	accent  lipgloss.Color // 选中/强调色
	cache   *artFrames     // 动画/置灰渲染缓存(懒构建)
}

// artFrames 预渲染的动画帧与静态置灰图,避免每帧重算。
type artFrames struct {
	framesLarge []string
	framesSmall []string
	dimLarge    string
	dimSmall    string
}

// renderArt 渲染图标。dim=true 为静态灰阶;否则返回循环动画的第 phase 帧
// (渐变相位沿对角线流动)。帧与置灰图首次使用时预渲染并缓存。
func (t *targetInfo) renderArt(small bool, dim bool, phase int) string {
	if t.cache == nil {
		t.cache = &artFrames{}
	}
	px, frames, dimStr := t.pxLarge, &t.cache.framesLarge, &t.cache.dimLarge
	if small {
		px, frames, dimStr = t.pxSmall, &t.cache.framesSmall, &t.cache.dimSmall
	}
	if len(*frames) == 0 {
		for i := 0; i < animFrames; i++ {
			*frames = append(*frames, renderHalfblock(px, t.grad, float64(i)/float64(animFrames)))
		}
	}
	if *dimStr == "" {
		*dimStr = renderHalfblock(px, gradDim, 0)
	}
	if dim {
		return *dimStr
	}
	return (*frames)[phase%len(*frames)]
}
