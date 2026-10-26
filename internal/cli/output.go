package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// writeJSON 输出单行 JSON(agent 解析友好)。
func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// shortTime 把可选时间渲染成 RFC3339 或空串。
func shortTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Local().Format(time.RFC3339)
}

// printTable 输出简单对齐的表格(仅人读路径使用)。
func printTable(w io.Writer, header []string, rows [][]string) {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = displayWidth(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths) && displayWidth(c) > widths[i] {
				widths[i] = displayWidth(c)
			}
		}
	}
	line := func(cells []string) string {
		parts := make([]string, 0, len(cells))
		for i, c := range cells {
			if i == len(cells)-1 {
				parts = append(parts, c)
				continue
			}
			pad := widths[i] - displayWidth(c)
			if pad < 0 {
				pad = 0
			}
			parts = append(parts, c+strings.Repeat(" ", pad))
		}
		return strings.TrimRight(strings.Join(parts, "  "), " ")
	}
	fmt.Fprintln(w, line(header))
	for _, r := range rows {
		fmt.Fprintln(w, line(r))
	}
}

// displayWidth 近似显示宽度:中日韩字符按 2 列算。
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x1100 && (r <= 0x115F ||
			(r >= 0x2E80 && r <= 0xA4CF) ||
			(r >= 0xAC00 && r <= 0xD7A3) ||
			(r >= 0xF900 && r <= 0xFAFF) ||
			(r >= 0xFE30 && r <= 0xFE6F) ||
			(r >= 0xFF00 && r <= 0xFF60) ||
			(r >= 0xFFE0 && r <= 0xFFE6)) {
			n += 2
			continue
		}
		n++
	}
	return n
}
