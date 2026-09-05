// 工具注册与接入契约。
//
// 新增一个目标软件的凭据管理:
//  1. 新建 tui_<key>.go,实现 Manager 接口(业务逻辑放独立 internal 包);
//  2. 定义 targetInfo(Key = CLI 预选名)与操作栏 chips;
//  3. 在 newRegistry() 追加一个构造调用。
//
// 首屏选择、根路由、底部操作栏、删除待确认/摇晃动画等全部由框架复用,
// 已有代码不需要改动。
package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// promptTimeout 删除待确认提示的自动消失时间。
const promptTimeout = 5 * time.Second

// Manager 一个目标软件的账号管理界面。
type Manager interface {
	Info() *targetInfo // 展示信息(名称/路径/图标/渐变/强调色)
	Chips() []chip     // 底部操作栏
	Init() tea.Cmd
	Update(tea.Msg) (Manager, tea.Cmd)
	View() string
}

// newRegistry 注册所有支持的工具。
func newRegistry() []Manager {
	return []Manager{NewChatgpt(), NewZcode()}
}

// HasTarget 报告 key 是否为已注册工具的 CLI 预选名。
func HasTarget(key string) bool {
	for _, m := range newRegistry() {
		if m.Info().Key == key {
			return true
		}
	}
	return false
}

// TargetKeys 返回全部已注册工具的 CLI 预选名(用于用法提示)。
func TargetKeys() []string {
	keys := make([]string, 0, 4)
	for _, m := range newRegistry() {
		keys = append(keys, m.Info().Key)
	}
	return keys
}

// targetSelectedMsg 首屏选择完成,携带注册表下标。
type targetSelectedMsg struct{ Index int }
