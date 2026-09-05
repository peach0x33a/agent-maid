// 官方图标 → 半块方块(halfblock)渐变渲染:每个终端字符行表示两行像素,
// "▀" 前景=上像素、背景=下像素;实心像素按位置沿对角线取渐变色,
// 边缘像素向底色插值,得到平滑的抗锯齿观感。
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func lipStyle(fg, bg string) lipgloss.Style {
	st := lipgloss.NewStyle()
	if fg != "" {
		st = st.Foreground(lipgloss.Color(fg))
	}
	if bg != "" {
		st = st.Background(lipgloss.Color(bg))
	}
	return st
}

// gradAt 在 stops 间做对角线渐变插值,x/y 为像素坐标。
func gradAt(stops []string, x, y, w, h int) string {
	tx, ty := float64(x)/float64(w-1), float64(y)/float64(h-1)
	t := (tx + ty) / 2
	return sampleStops(stops, t)
}

func sampleStops(stops []string, t float64) string {
	if len(stops) == 1 {
		return stops[0]
	}
	seg := float64(len(stops)-1) * t
	i := int(seg)
	if i >= len(stops)-1 {
		return stops[len(stops)-1]
	}
	return lerpColor(stops[i], stops[i+1], seg-float64(i))
}

// lerpColor 十六进制颜色线性插值,#RRGGBB。
func lerpColor(a, b string, t float64) string {
	ar, ag, ab := hexRGB(a)
	br, bg, bb := hexRGB(b)
	mix := func(x, y float64) int { return int(x + (y-x)*t + 0.5) }
	return fmt.Sprintf("#%02X%02X%02X", mix(ar, br), mix(ag, bg), mix(ab, bb))
}

func hexRGB(s string) (r, g, b float64) {
	var v uint32
	for _, c := range s[1:] {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= uint32(c - '0')
		case c >= 'A' && c <= 'F':
			v |= uint32(c - 'A' + 10)
		case c >= 'a' && c <= 'f':
			v |= uint32(c - 'a' + 10)
		}
	}
	return float64(v >> 16 & 0xFF), float64(v >> 8 & 0xFF), float64(v & 0xFF)
}

// inkOf 取像素 ink 等级(0-9)归一化。
func inkOf(px []string, x, y int) float64 {
	if y < 0 || y >= len(px) || x < 0 || x >= len(px[y]) {
		return 0
	}
	return float64(px[y][x]-'0') / 9.0
}

// renderHalfblock 把 ink 矩阵渲染为渐变半块方块。
// px 为 w×h 像素 ink 等级矩阵(0-9),渲染为 w 列 × h/2 行字符。
// 低于 inkOn 的像素直接不画(不留暗色边缘),实心像素取纯渐变色。
func renderHalfblock(px []string, stops []string) string {
	const inkOn = 0.3
	w := len(px[0])
	h := len(px)
	var b strings.Builder
	for row := 0; row*2 < h; row++ {
		for x := 0; x < w; x++ {
			up, down := inkOf(px, x, row*2), inkOf(px, x, row*2+1)
			upOn, downOn := up >= inkOn, down >= inkOn
			switch {
			case !upOn && !downOn:
				b.WriteString(" ")
			case upOn && downOn:
				fg := gradAt(stops, x, row*2, w, h)
				bg := gradAt(stops, x, row*2+1, w, h)
				b.WriteString(lipStyle(fg, bg).Render("▀"))
			case upOn:
				fg := gradAt(stops, x, row*2, w, h)
				b.WriteString(lipStyle(fg, "").Render("▀"))
			default: // 仅下半实:前景与背景同色,只显示下半块
				bg := gradAt(stops, x, row*2+1, w, h)
				b.WriteString(lipStyle(bg, bg).Render("▀"))
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
