// 无头子命令的分发:把解析好的参数交给各目标的实现。
package cli

import "fmt"

func runList(pos []string, env Env) int {
	target, code, ok := env.needTarget(pos, "list")
	if !ok {
		return code
	}
	if len(pos) > 1 {
		return env.fail(CodeUsage, "list 只需要一个目标", usageCLI())
	}
	switch target {
	case "chatgpt":
		return gptList(env)
	case "zcode":
		return zcodeList(env)
	case "antigravity":
		return agyList(env)
	}
	return env.fail(CodeUsage, "未知目标 "+target, "")
}

func runCurrent(pos []string, env Env) int {
	target, code, ok := env.needTarget(pos, "current")
	if !ok {
		return code
	}
	switch target {
	case "chatgpt":
		return gptCurrent(env)
	case "zcode":
		return zcodeCurrent(env)
	case "antigravity":
		return agyCurrent(env)
	}
	return env.fail(CodeUsage, "未知目标 "+target, "")
}

func runUse(pos []string, env Env) int {
	target, code, ok := env.needTarget(pos, "use")
	if !ok {
		return code
	}
	if len(pos) < 2 {
		return env.fail(CodeUsage, "use 需要一个账号/槽位名(用 list 查看)", usageCLI())
	}
	name := pos[1]
	switch target {
	case "chatgpt":
		return gptUse(env, name)
	case "zcode":
		return zcodeUse(env, name)
	case "antigravity":
		return agyUse(env, name)
	}
	return env.fail(CodeUsage, "未知目标 "+target, "")
}

func runSave(pos []string, env Env) int {
	target, code, ok := env.needTarget(pos, "save")
	if !ok {
		return code
	}
	name := ""
	if len(pos) > 1 {
		name = pos[1]
	}
	switch target {
	case "chatgpt":
		return gptSave(env, name)
	case "zcode":
		return zcodeSave(env, name)
	case "antigravity":
		return agySave(env, name)
	}
	return env.fail(CodeUsage, "未知目标 "+target, "")
}

func runRemove(pos []string, env Env) int {
	target, code, ok := env.needTarget(pos, "rm")
	if !ok {
		return code
	}
	if len(pos) < 2 {
		return env.fail(CodeUsage, "rm 需要一个账号/槽位名(用 list 查看)", usageCLI())
	}
	name := pos[1]
	switch target {
	case "chatgpt":
		return gptRemove(env, name)
	case "zcode":
		return zcodeRemove(env, name)
	case "antigravity":
		return agyRemove(env, name)
	}
	return env.fail(CodeUsage, "未知目标 "+target, "")
}

func runImport(pos []string, env Env) int {
	target, code, ok := env.needTarget(pos, "import")
	if !ok {
		return code
	}
	if target != "chatgpt" {
		return env.fail(CodeUsage,
			fmt.Sprintf("%s 没有导入功能:账号来自本机已登录的凭据,先登录再 `save %s <名称>`", targetLabel(target), target), "")
	}
	if len(pos) < 2 {
		return env.fail(CodeUsage, "import 需要一个文件路径,或用 - 从 stdin 读取", usageCLI())
	}
	return gptImport(env, pos[1])
}

func runLogout(pos []string, env Env) int {
	target, code, ok := env.needTarget(pos, "logout")
	if !ok {
		return code
	}
	switch target {
	case "chatgpt":
		return gptLogout(env)
	case "zcode":
		return zcodeLogout(env)
	case "antigravity":
		return agyLogout(env)
	}
	return env.fail(CodeUsage, "未知目标 "+target, "")
}
