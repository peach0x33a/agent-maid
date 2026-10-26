// Package cli 提供 agent-maid 的无头子命令:给 AI agent 与脚本用。
//
// 设计原则:
//   - 每个命令都能输出 --json,便于 agent 解析(field 名稳定,不随文案变);
//   - 默认输出人可读的一行/一表,不进入 TUI,不需要交互终端;
//   - 写操作一律"显式指定目标",不做隐式猜测,并复用 TUI 的同一套业务层
//     (gptapp / zcodeapp / antigravity),避免行为分叉。
package cli

import (
	"fmt"
	"io"
	"strings"
)

// Env 命令执行环境。
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	JSON   bool
	// Force 允许 use 在"当前账号未保存"时直接切换(--force)。
	// 默认拒绝:那样会留下一个没人认得的 auto-<时间戳> 备份槽位。
	Force bool
}

// Usage 返回无头子命令的用法说明(供 main 拼进 --help)。
func Usage() string { return usageCLI() }

// usageCLI 无头子命令的用法说明。
func usageCLI() string {
	return `无头命令(给 AI agent / 脚本用;不需要交互终端):

  agent-maid list   <目标>            列出账号/槽位,标出正在使用的那一个
  agent-maid current <目标>           显示当前生效的凭据
  agent-maid use    <目标> <名称> [--force]
                                      切换/激活到指定账号;当前账号未保存时
                                      默认拒绝,--force 才会先自动备份再切换
  agent-maid save   <目标> [名称]     把当前生效的凭据保存为账号/槽位
  agent-maid rm     <目标> <名称>     删除账号/槽位(移入回收站)
  agent-maid import <目标> <文件|->   导入凭据(chatgpt 支持;从 stdin 读需 -)
  agent-maid logout <目标>            登出:清空本地凭据,账号保留

  目标: chatgpt | zcode | antigravity

每个命令都支持 --json(放在任意位置),输出带 type/ok 字段的 JSON 对象,
失败时退出码非 0 并把原因写进 error 字段。
`
}

// IsCommand 报告 argv 的首个参数是否为一个无头子命令。
// 用于在 main 里把 CLI 与 TUI 的启动参数区分开(两者都可能出现
// chatgpt/zcode 这类词,只有命令名才进 CLI 分支)。
func IsCommand(args []string) bool {
	// 允许 --json / --force 出现在最前面(agent 常见的写法:cmd --json sub ...)
	for _, a := range args {
		if a == "--json" || a == "--force" {
			continue
		}
		switch a {
		case "list", "ls", "current", "cur", "use", "switch", "activate",
			"save", "rm", "remove", "delete", "import", "add", "logout", "signout":
			return true
		}
		return false
	}
	return false
}

// Run 执行一个无头子命令。返回值即进程退出码。
func Run(args []string, env Env) int {
	if len(args) == 0 {
		fmt.Fprint(env.Stdout, usageCLI())
		return 0
	}
	// --json / --force 可出现在任意位置(包括最前面)
	var positional []string
	jsonOut := env.JSON
	force := env.Force
	for _, a := range args {
		switch a {
		case "--json":
			jsonOut = true
			continue
		case "--force":
			force = true
			continue
		}
		positional = append(positional, a)
	}
	env.JSON = jsonOut
	env.Force = force
	if len(positional) == 0 {
		fmt.Fprint(env.Stdout, usageCLI())
		return 0
	}
	// 剩下以 - 开头的一律视为未知选项:否则拼错的 --force 会被当成账号名,
	// 报一个"找不到账号"的误导性错误。
	for _, a := range positional {
		if strings.HasPrefix(a, "-") && a != "-" {
			return env.fail(CodeUsage, fmt.Sprintf("未知选项 %q", a), usageCLI())
		}
	}

	cmd := positional[0]
	rest := positional[1:]
	if cmd == "help" {
		fmt.Fprint(env.Stdout, usageCLI())
		return 0
	}

	switch cmd {
	case "list", "ls":
		return runList(rest, env)
	case "current", "cur":
		return runCurrent(rest, env)
	case "use", "switch", "activate":
		return runUse(rest, env)
	case "save":
		return runSave(rest, env)
	case "rm", "remove", "delete":
		return runRemove(rest, env)
	case "import", "add":
		return runImport(rest, env)
	case "logout", "signout":
		return runLogout(rest, env)
	case "help":
		fmt.Fprint(env.Stdout, usageCLI())
		return 0
	default:
		return env.fail(CodeUsage, fmt.Sprintf("未知命令 %q", cmd), usageCLI())
	}
}

// 退出码:脚本按需区分。
const (
	CodeOK      = 0
	CodeFailure = 1 // 操作失败(文件读写、找不到目标……)
	CodeUsage   = 2 // 用法错误(参数个数、未知目标)
)

// payload 所有 JSON 输出的共同外壳。
type payload struct {
	Type  string `json:"type"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// emitJSON 输出一个 JSON 结果。
func (e Env) emitJSON(v any) {
	writeJSON(e.Stdout, v)
}

// fail 输出错误并返回退出码。
func (e Env) fail(code int, msg, hint string) int {
	if e.JSON {
		writeJSON(e.Stdout, map[string]any{
			"type":  "error",
			"ok":    false,
			"error": strings.TrimSpace(msg),
		})
	} else {
		fmt.Fprintln(e.Stderr, "错误: "+strings.TrimSpace(msg))
		if hint != "" {
			fmt.Fprint(e.Stderr, "\n"+hint)
		}
	}
	return code
}

// failUnsaved 当前账号未保存时拒绝切换。这类失败要给出可执行的下一步,
// 而不是一句"失败"——调用方(尤其 agent)往往不知道要先去 save。
func (e Env) failUnsaved(target, detail string) int {
	hint := fmt.Sprintf("先把它存下来:\n  agent-maid save %s <名称>\n"+
		"确认不需要保留当前账号时,再加 --force 直接切换(会自动备份到 auto-<时间戳> 槽位)。", target)
	if e.JSON {
		writeJSON(e.Stdout, map[string]any{
			"type":           "error",
			"ok":             false,
			"error":          detail,
			"reason":         "unsaved_current",
			"target":         target,
			"needs":          "save",
			"suggested_save": fmt.Sprintf("agent-maid save %s <name>", target),
			"force_flag":     "--force",
		})
	} else {
		fmt.Fprintln(e.Stderr, "错误: "+detail)
		fmt.Fprint(e.Stderr, hint+"\n")
	}
	return CodeFailure
}

// needTarget 校验并解析目标名。
func (e Env) needTarget(positional []string, cmd string) (string, int, bool) {
	if len(positional) == 0 {
		return "", e.fail(CodeUsage, cmd+" 缺少目标", usageCLI()), false
	}
	target := normalizeTarget(positional[0])
	if target == "" {
		return "", e.fail(CodeUsage, fmt.Sprintf("未知目标 %q,可用: chatgpt | zcode | antigravity", positional[0]), ""), false
	}
	return target, 0, true
}

// normalizeTarget 把用户输入规范化成内部 key;未知返回空串。
func normalizeTarget(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "chatgpt", "gpt", "codex":
		return "chatgpt"
	case "zcode", "z":
		return "zcode"
	case "antigravity", "anti", "ag", "agy":
		return "antigravity"
	}
	return ""
}

// targetLabel 展示用的目标名。
func targetLabel(key string) string {
	switch key {
	case "chatgpt":
		return "ChatGPT"
	case "zcode":
		return "ZCode"
	case "antigravity":
		return "Antigravity"
	}
	return key
}
