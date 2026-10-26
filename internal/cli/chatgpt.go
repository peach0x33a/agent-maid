// ChatGPT 的无头子命令:list / current / use / save / rm / import / logout。
package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"agent-maid/internal/gptapp"
	"agent-maid/internal/gptauth"
	"agent-maid/internal/gptstore"
)

// accountView 列表里的一条账号(JSON 输出结构)。
type accountView struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Email       string `json:"email,omitempty"`
	AccountID   string `json:"account_id,omitempty"`
	Plan        string `json:"plan,omitempty"`
	Source      string `json:"source,omitempty"`
	Refreshable bool   `json:"refreshable"`
	InUse       bool   `json:"in_use"`
	LastUsedAt  string `json:"last_used_at,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

// gptAccounts 取账号库与当前生效凭据。
func gptAccounts(svc *gptapp.Service) ([]*gptstore.Account, *gptauth.Current, error) {
	accounts, _, _, err := svc.List()
	if err != nil {
		return nil, nil, err
	}
	current, _, err := svc.Current()
	if err != nil {
		return nil, nil, err
	}
	return accounts, current, nil
}

// inUse 账号是否就是官方 auth.json 当前生效的那个(以 account_id 为准)。
func inUse(a *gptstore.Account, current *gptauth.Current) bool {
	return current != nil && a.AccountID != "" && a.AccountID == current.AccountID
}

func gptList(env Env) int {
	svc := gptapp.New()
	accounts, current, err := gptAccounts(svc)
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	views := make([]accountView, 0, len(accounts))
	for _, a := range accounts {
		v := accountView{
			ID:          a.ID,
			Label:       a.Label,
			Email:       a.Email,
			AccountID:   a.AccountID,
			Plan:        a.PlanType,
			Source:      a.SourceFormat,
			Refreshable: a.Refreshable,
			InUse:       inUse(a, current),
			LastUsedAt:  shortTime(a.LastUsedAt),
			ExpiresAt:   shortTime(a.AccessExpiresAt),
		}
		views = append(views, v)
	}
	if env.JSON {
		cv := gptCurrentView(current)
		if cv != nil {
			cv.InLibrary = currentInLibrary(accounts, current)
		}
		env.emitJSON(struct {
			payload
			Target   string        `json:"target"`
			Total    int           `json:"total"`
			Accounts []accountView `json:"accounts"`
			Current  *currentView  `json:"current,omitempty"`
		}{payload{"list", true, ""}, "chatgpt", len(views), views, cv})
		return CodeOK
	}
	if len(views) == 0 {
		fmt.Fprintln(env.Stdout, "账号库为空。用 `agent-maid import chatgpt <文件|->` 导入,或先登录官方端再 `agent-maid save chatgpt`。")
		return CodeOK
	}
	rows := make([][]string, 0, len(views))
	for _, v := range views {
		mark := ""
		if v.InUse {
			mark = "●使用中"
		}
		if !v.Refreshable {
			mark += " ⚠无refresh_token"
		}
		rows = append(rows, []string{v.Label, orDash(v.Plan), orDash(v.AccountID), v.ID, mark})
	}
	printTable(env.Stdout, []string{"账号", "套餐", "account_id", "id", "状态"}, rows)
	return CodeOK
}

// currentInLibrary 当前凭据的 account_id 是否在账号库里。
func currentInLibrary(accounts []*gptstore.Account, current *gptauth.Current) bool {
	if current == nil || current.AccountID == "" {
		return false
	}
	for _, a := range accounts {
		if a.AccountID == current.AccountID {
			return true
		}
	}
	return false
}

// currentView 当前生效凭据的 JSON 结构。
type currentView struct {
	Exists      bool   `json:"exists"`
	AuthMode    string `json:"auth_mode,omitempty"`
	Email       string `json:"email,omitempty"`
	AccountID   string `json:"account_id,omitempty"`
	Path        string `json:"path,omitempty"`
	LastRefresh string `json:"last_refresh,omitempty"`
	InLibrary   bool   `json:"in_library"`
}

func gptCurrentView(current *gptauth.Current) *currentView {
	if current == nil {
		return nil
	}
	home, _ := gptauth.ResolveHome("")
	v := &currentView{
		Exists:      current.HasTokens || current.AuthMode != "",
		AuthMode:    current.AuthMode,
		Email:       current.Email,
		AccountID:   current.AccountID,
		LastRefresh: current.LastRefresh,
	}
	if home != "" {
		v.Path = gptauth.AuthFilePath(home)
	}
	return v
}

func gptCurrent(env Env) int {
	svc := gptapp.New()
	current, home, err := svc.Current()
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	inLibrary := false
	if accounts, _, _, lerr := svc.List(); lerr == nil {
		inLibrary = currentInLibrary(accounts, current)
	}
	view := gptCurrentView(current)
	if view != nil {
		view.InLibrary = inLibrary
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target  string       `json:"target"`
			Current *currentView `json:"current"`
		}{payload{"current", true, ""}, "chatgpt", view})
		return CodeOK
	}
	if view == nil || !view.Exists {
		fmt.Fprintf(env.Stdout, "当前未登录 Codex(%s 不存在)\n", gptauth.AuthFilePath(home))
		return CodeOK
	}
	fmt.Fprintf(env.Stdout, "认证方式: %s\n", gptapp.PrettyAuthMode(view.AuthMode))
	fmt.Fprintf(env.Stdout, "邮箱:     %s\n", orDash(view.Email))
	fmt.Fprintf(env.Stdout, "account:  %s\n", orDash(view.AccountID))
	fmt.Fprintf(env.Stdout, "账号库:   %s\n", map[bool]string{true: "已保存", false: "未保存(可用 save 收录)"}[inLibrary])
	fmt.Fprintf(env.Stdout, "文件:     %s\n", view.Path)
	return CodeOK
}

func gptUse(env Env, name string) int {
	res, err := gptapp.New().ActivateWith(name, gptapp.ActivateOptions{RequireSaved: !env.Force})
	var unsaved *gptapp.UnsavedCurrentError
	if errors.As(err, &unsaved) {
		return env.failUnsaved("chatgpt", unsaved.Error())
	}
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target  string       `json:"target"`
			Account *accountView `json:"account"`
			Path    string       `json:"path"`
		}{payload{"use", true, ""}, "chatgpt", &accountView{
			ID: res.Account.ID, Label: res.Account.Label, Email: res.Account.Email,
			AccountID: res.Account.AccountID, Plan: res.Account.PlanType,
			Source: res.Account.SourceFormat, Refreshable: res.Account.Refreshable,
			InUse: true, LastUsedAt: shortTime(res.Account.LastUsedAt),
		}, res.Path})
		return CodeOK
	}
	fmt.Fprintf(env.Stdout, "已激活 %s,已写入 %s\n", res.Account.Label, res.Path)
	return CodeOK
}

func gptSave(env Env, name string) int {
	res, err := gptapp.New().SaveCurrent(name)
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target  string       `json:"target"`
			Updated bool         `json:"updated"`
			Account *accountView `json:"account"`
		}{payload{"save", true, ""}, "chatgpt", res.Updated, &accountView{
			ID: res.Account.ID, Label: res.Account.Label, Email: res.Account.Email,
			AccountID: res.Account.AccountID, Plan: res.Account.PlanType,
			Source: res.Account.SourceFormat, Refreshable: res.Account.Refreshable,
			// save 就是把当前生效的凭据收进库里,所以它此刻一定是使用中的账号
			InUse: res.Account.AccountID != "",
		}})
		return CodeOK
	}
	if res.Updated {
		fmt.Fprintf(env.Stdout, "账号库里已有该账号,已更新: %s\n", res.Account.Label)
	} else {
		fmt.Fprintf(env.Stdout, "已保存到账号库: %s\n", res.Account.Label)
	}
	return CodeOK
}

func gptRemove(env Env, name string) int {
	trash, err := gptapp.New().Remove(name)
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target string `json:"target"`
			Trash  string `json:"trash"`
		}{payload{"rm", true, ""}, "chatgpt", trash})
		return CodeOK
	}
	fmt.Fprintf(env.Stdout, "已移入回收站: %s(在 TUI 里退出前可按 z 撤销;本次会话结束后正式删除)\n", name)
	return CodeOK
}

func gptImport(env Env, arg string) int {
	data, source, err := gptapp.ReadImportFile(arg, env.Stdin)
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	res, err := gptapp.New().Import(data, source)
	if err != nil {
		if errors.Is(err, gptapp.ErrNoAccounts) || errors.Is(err, gptapp.ErrConvertFailed) {
			if env.JSON {
				env.emitJSON(struct {
					payload
					Target  string `json:"target"`
					Found   int    `json:"found"`
					BadDocs int    `json:"bad_docs"`
				}{payload{"import", false, err.Error()}, "chatgpt", res.Total, res.BadDocs})
				return CodeFailure
			}
			return env.fail(CodeFailure, err.Error(), "")
		}
		return env.fail(CodeFailure, err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target   string `json:"target"`
			Imported int    `json:"imported"`
			Skipped  int    `json:"skipped"`
			BadDocs  int    `json:"bad_docs"`
		}{payload{"import", true, ""}, "chatgpt", res.Imported, res.Skipped, res.BadDocs})
		return CodeOK
	}
	fmt.Fprintf(env.Stdout, "已导入 %d 个账号", res.Imported)
	if res.Skipped > 0 {
		fmt.Fprintf(env.Stdout, ",跳过 %d 项", res.Skipped)
	}
	if res.BadDocs > 0 {
		fmt.Fprintf(env.Stdout, ",忽略 %d 个无效文档", res.BadDocs)
	}
	fmt.Fprintln(env.Stdout, "。用 `agent-maid use chatgpt <名称>` 激活。")
	return CodeOK
}

func gptLogout(env Env) int {
	res, err := gptapp.New().Logout()
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target string `json:"target"`
			Backup string `json:"auto_saved,omitempty"`
		}{payload{"logout", true, ""}, "chatgpt", res.Backup})
		return CodeOK
	}
	if res.Backup != "" {
		fmt.Fprintf(env.Stdout, "已登出 Codex(当前凭据已先存入账号库: %s)\n", res.Backup)
	} else {
		fmt.Fprintln(env.Stdout, "已登出 Codex(账号保留在账号库,服务端令牌未吊销)")
	}
	return CodeOK
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// nowLocal 测试与展示用的当前时间。
func nowLocal() time.Time { return time.Now() }
