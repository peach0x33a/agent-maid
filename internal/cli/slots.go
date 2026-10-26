// ZCode 与 Antigravity(agy)的无头子命令:两者都是"槽位"模型,共用一套实现。
package cli

import (
	"errors"
	"fmt"

	"agent-maid/internal/antigravity"
	"agent-maid/internal/zcode"
	"agent-maid/internal/zcodeapp"
	"agent-maid/internal/zcodestore"
)

// slotView 槽位的 JSON 结构(ZCode / agy 共用字段名)。
type slotView struct {
	Name        string `json:"name"`
	Account     string `json:"account,omitempty"`
	Email       string `json:"email,omitempty"`
	InUse       bool   `json:"in_use"`
	LastUsedAt  string `json:"last_used_at,omitempty"`
	Refreshable bool   `json:"refreshable"`
	Expired     bool   `json:"expired,omitempty"`
	Warning     string `json:"warning,omitempty"`
}

// ───────────────────────── ZCode ─────────────────────────

func zcodeSlots() ([]zcodestore.Slot, zcodeapp.CurrentInfo, error) {
	svc := zcodeapp.New()
	slots, err := svc.Store.List()
	if err != nil {
		return nil, zcodeapp.CurrentInfo{}, err
	}
	return slots, svc.Current(), nil
}

func zcodeSlotViews() ([]slotView, zcodeapp.CurrentInfo, error) {
	slots, current, err := zcodeSlots()
	if err != nil {
		return nil, current, err
	}
	views := make([]slotView, 0, len(slots))
	for _, s := range slots {
		v := slotView{
			Name:        s.Name,
			Account:     zcodeAccountLabel(s),
			InUse:       current.Exists && zcode.EqualCredentials(current.Creds, s.Credentials),
			LastUsedAt:  shortTime(s.LastUsedAt),
			Refreshable: true,
		}
		if s.KeyFingerprint != zcode.KeyFingerprint(zcode.DeriveKey(zcode.CredentialSecret())) {
			v.Warning = "跨机/跨密钥:槽位可能无法解密"
		}
		views = append(views, v)
	}
	return views, current, nil
}

func zcodeAccountLabel(s zcodestore.Slot) string {
	parts := []string{}
	if s.Meta.Provider != "" {
		parts = append(parts, s.Meta.Provider)
	}
	name := s.Meta.DisplayName
	if name == "" {
		name = s.Meta.Username
	}
	if name != "" {
		parts = append(parts, name)
	}
	if len(parts) == 0 {
		return "未知账号"
	}
	return joinSlash(parts)
}

func joinSlash(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " / "
		}
		out += p
	}
	return out
}

func zcodeList(env Env) int {
	views, current, err := zcodeSlotViews()
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	currentName := ""
	if current.Exists {
		currentName = zcodeAccountLabel(zcodestore.Slot{Meta: current.Meta})
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target  string     `json:"target"`
			Total   int        `json:"total"`
			Slots   []slotView `json:"slots"`
			Current string     `json:"current,omitempty"`
			Running bool       `json:"running"`
		}{payload{"list", true, ""}, "zcode", len(views), views, currentName, zcode.ZCodeRunning()})
		return CodeOK
	}
	if len(views) == 0 {
		fmt.Fprintln(env.Stdout, "槽位为空。先让 ZCode 登录,cwd 后用 `agent-maid save zcode <名称>` 保存。")
		return CodeOK
	}
	rows := make([][]string, 0, len(views))
	for _, v := range views {
		mark := ""
		if v.InUse {
			mark = "●使用中"
		}
		if v.Warning != "" {
			mark += " ⚠" + v.Warning
		}
		rows = append(rows, []string{v.Name, v.Account, v.LastUsedAt, mark})
	}
	printTable(env.Stdout, []string{"槽位", "账号", "最后使用", "状态"}, rows)
	return CodeOK
}

func zcodeCurrent(env Env) int {
	_, current, err := zcodeSlots()
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	label := ""
	if current.Exists {
		label = zcodeAccountLabel(zcodestore.Slot{Meta: current.Meta})
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target  string `json:"target"`
			Exists  bool   `json:"exists"`
			Account string `json:"account,omitempty"`
			Decrypt bool   `json:"decrypt_ok"`
			Path    string `json:"path"`
			Running bool   `json:"running"`
		}{payload{"current", true, ""}, "zcode", current.Exists, label, current.Decrypt, zcode.CredentialsPath(), zcode.ZCodeRunning()})
		return CodeOK
	}
	if !current.Exists {
		fmt.Fprintf(env.Stdout, "当前未登录 ZCode(%s 不存在)\n", zcode.CredentialsPath())
		return CodeOK
	}
	fmt.Fprintf(env.Stdout, "当前账号: %s\n", orDash(label))
	fmt.Fprintf(env.Stdout, "文件:     %s\n", zcode.CredentialsPath())
	if !current.Decrypt {
		fmt.Fprintln(env.Stdout, "注意: 无法解密(密钥不匹配,可能设置了 ZCODE_CREDENTIAL_SECRET)")
	}
	return CodeOK
}

func zcodeUse(env Env, name string) int {
	svc := zcodeapp.New()
	if !svc.Store.Has(name) {
		return env.fail(CodeFailure, fmt.Sprintf("槽位 %q 不存在(用 list 查看)", name), "")
	}
	running := zcode.ZCodeRunning()
	backup, err := svc.ApplyWith(name, zcodeapp.ApplyOptions{RequireSaved: !env.Force})
	var unsaved *zcodeapp.UnsavedCurrentError
	if errors.As(err, &unsaved) {
		return env.failUnsaved("zcode", unsaved.Error())
	}
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target  string `json:"target"`
			Slot    string `json:"slot"`
			Backup  string `json:"auto_backup,omitempty"`
			Running bool   `json:"zcode_running"`
			Warning string `json:"warning,omitempty"`
		}{payload{"use", true, ""}, "zcode", name, backup, running, runningWarning(running, "ZCode")})
		return CodeOK
	}
	fmt.Fprintf(env.Stdout, "已切换到槽位 %s", name)
	if backup != "" {
		fmt.Fprintf(env.Stdout, "(原账号已自动备份到 %s)", backup)
	}
	fmt.Fprintln(env.Stdout)
	if running {
		fmt.Fprintln(env.Stdout, "注意: ZCode 正在运行,退出前可能把凭据回写覆盖,建议重启 ZCode。")
	}
	return CodeOK
}

func runningWarning(running bool, app string) string {
	if !running {
		return ""
	}
	return app + " 正在运行:切换后建议尽快重启,避免运行中的凭据回写覆盖"
}

func zcodeSave(env Env, name string) int {
	svc := zcodeapp.New()
	current := svc.Current()
	if !current.Exists {
		return env.fail(CodeFailure, "没有找到 "+zcode.CredentialsPath()+",请先在 ZCode 里登录", "")
	}
	slot, err := svc.SaveCurrent(name, false)
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target string   `json:"target"`
			Slot   slotView `json:"slot"`
		}{payload{"save", true, ""}, "zcode", slotView{Name: slot.Name, Account: zcodeAccountLabel(slot)}})
		return CodeOK
	}
	fmt.Fprintf(env.Stdout, "已保存到槽位 %s(%s)\n", slot.Name, zcodeAccountLabel(slot))
	return CodeOK
}

func zcodeRemove(env Env, name string) int {
	trash, err := zcodeapp.New().Store.DeleteToTrash(name)
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target string `json:"target"`
			Trash  string `json:"trash"`
		}{payload{"rm", true, ""}, "zcode", trash})
		return CodeOK
	}
	fmt.Fprintf(env.Stdout, "已删除槽位 %s(移入回收站)\n", name)
	return CodeOK
}

func zcodeLogout(env Env) int {
	svc := zcodeapp.New()
	if !svc.Current().Exists {
		return env.fail(CodeFailure, "当前没有登录凭据,无需登出", "")
	}
	backup, err := svc.BackupCurrentIfUncovered()
	if err != nil {
		return env.fail(CodeFailure, "自动备份失败: "+err.Error(), "")
	}
	if err := zcode.ClearCredentials(); err != nil {
		return env.fail(CodeFailure, "清空凭据失败: "+err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target string `json:"target"`
			Backup string `json:"auto_backup,omitempty"`
		}{payload{"logout", true, ""}, "zcode", backup})
		return CodeOK
	}
	if backup != "" {
		fmt.Fprintf(env.Stdout, "已登出 ZCode(原账号已自动备份到槽位 %s)\n", backup)
	} else {
		fmt.Fprintln(env.Stdout, "已登出 ZCode(账号已保存在槽位)")
	}
	return CodeOK
}

// ───────────────────────── Antigravity (agy) ─────────────────────────

func agySlotViews() ([]slotView, antigravity.CurrentInfo, error) {
	svc := antigravity.New()
	slots, err := svc.Store.List()
	if err != nil {
		return nil, antigravity.CurrentInfo{}, err
	}
	current := svc.Current()
	now := nowLocal()
	views := make([]slotView, 0, len(slots))
	for _, s := range slots {
		v := slotView{
			Name:        s.Name,
			Email:       s.Email,
			InUse:       current.Exists && antigravity.EqualToken(current.Token, s.Token),
			LastUsedAt:  shortTime(s.LastUsedAt),
			Refreshable: s.State.Refreshable,
			Expired:     s.State.Expired(now),
		}
		if v.Expired && !v.Refreshable {
			v.Warning = "已过期且无 refresh_token,需在 agy 重新登录"
		}
		views = append(views, v)
	}
	return views, current, nil
}

func agyList(env Env) int {
	views, current, err := agySlotViews()
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target  string     `json:"target"`
			Total   int        `json:"total"`
			Slots   []slotView `json:"slots"`
			Current string     `json:"current,omitempty"`
			Running bool       `json:"running"`
			Path    string     `json:"path"`
		}{payload{"list", true, ""}, "antigravity", len(views), views, current.Email, antigravity.Running(), antigravity.TokenPath()})
		return CodeOK
	}
	if len(views) == 0 {
		fmt.Fprintln(env.Stdout, "槽位为空。先在 agy 里登录,再 `agent-maid save antigravity <名称>` 保存。")
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
		if v.Warning != "" {
			mark += " ⚠" + v.Warning
		}
		rows = append(rows, []string{v.Name, v.Email, v.LastUsedAt, mark})
	}
	printTable(env.Stdout, []string{"槽位", "邮箱", "最后使用", "状态"}, rows)
	return CodeOK
}

func agyCurrent(env Env) int {
	svc := antigravity.New()
	current := svc.Current()
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target      string `json:"target"`
			Exists      bool   `json:"exists"`
			Email       string `json:"email,omitempty"`
			AuthMethod  string `json:"auth_method,omitempty"`
			Refreshable bool   `json:"refreshable"`
			ExpiresAt   string `json:"expires_at,omitempty"`
			Expired     bool   `json:"expired"`
			Path        string `json:"path"`
			Running     bool   `json:"running"`
		}{
			payload{"current", true, ""}, "antigravity", current.Exists, current.Email, current.AuthMode,
			current.State.Refreshable, shortTime(current.State.ExpiresAt),
			current.State.Expired(nowLocal()), antigravity.TokenPath(), antigravity.Running(),
		})
		return CodeOK
	}
	if !current.Exists {
		fmt.Fprintf(env.Stdout, "当前未登录 agy(%s 不存在)\n", antigravity.TokenPath())
		return CodeOK
	}
	fmt.Fprintf(env.Stdout, "邮箱:     %s\n", orDash(current.Email))
	fmt.Fprintf(env.Stdout, "认证方式: %s\n", orDash(current.AuthMode))
	if current.State.ExpiresAt != nil {
		fmt.Fprintf(env.Stdout, "有效期至: %s\n", shortTime(current.State.ExpiresAt))
	}
	if !current.State.Refreshable {
		fmt.Fprintln(env.Stdout, "注意: 凭据没有 refresh_token,过期后需在 agy 重新登录")
	}
	fmt.Fprintf(env.Stdout, "文件:     %s\n", antigravity.TokenPath())
	return CodeOK
}

func agyUse(env Env, name string) int {
	svc := antigravity.New()
	if !svc.Store.Has(name) {
		return env.fail(CodeFailure, fmt.Sprintf("槽位 %q 不存在(用 list 查看)", name), "")
	}
	running := antigravity.Running()
	backup, err := svc.ApplyWith(name, antigravity.ApplyOptions{RequireSaved: !env.Force})
	var unsaved *antigravity.UnsavedCurrentError
	if errors.As(err, &unsaved) {
		return env.failUnsaved("antigravity", unsaved.Error())
	}
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target  string `json:"target"`
			Slot    string `json:"slot"`
			Backup  string `json:"auto_backup,omitempty"`
			Running bool   `json:"agy_running"`
			Warning string `json:"warning,omitempty"`
		}{payload{"use", true, ""}, "antigravity", name, backup, running, runningWarning(running, "agy")})
		return CodeOK
	}
	fmt.Fprintf(env.Stdout, "已切换到槽位 %s", name)
	if backup != "" {
		fmt.Fprintf(env.Stdout, "(原账号已自动备份到 %s)", backup)
	}
	fmt.Fprintln(env.Stdout)
	if running {
		fmt.Fprintln(env.Stdout, "注意: agy 正在运行,退出前可能把凭据回写覆盖,建议重启 agy。")
	}
	return CodeOK
}

func agySave(env Env, name string) int {
	svc := antigravity.New()
	if !svc.Current().Exists {
		return env.fail(CodeFailure, "没有找到 "+antigravity.TokenPath()+",请先在 agy 里登录", "")
	}
	slot, err := svc.SaveCurrent(name, false)
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target string   `json:"target"`
			Slot   slotView `json:"slot"`
		}{payload{"save", true, ""}, "antigravity", slotView{
			Name: slot.Name, Email: slot.Email, Refreshable: slot.State.Refreshable,
		}})
		return CodeOK
	}
	fmt.Fprintf(env.Stdout, "已保存到槽位 %s(%s)\n", slot.Name, orDash(slot.Email))
	return CodeOK
}

func agyRemove(env Env, name string) int {
	trash, err := antigravity.New().Store.DeleteToTrash(name)
	if err != nil {
		return env.fail(CodeFailure, err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target string `json:"target"`
			Trash  string `json:"trash"`
		}{payload{"rm", true, ""}, "antigravity", trash})
		return CodeOK
	}
	fmt.Fprintf(env.Stdout, "已删除槽位 %s(移入回收站)\n", name)
	return CodeOK
}

func agyLogout(env Env) int {
	svc := antigravity.New()
	if !svc.Current().Exists {
		return env.fail(CodeFailure, "当前没有登录凭据,无需登出", "")
	}
	backup, err := svc.BackupCurrentIfUncovered()
	if err != nil {
		return env.fail(CodeFailure, "自动备份失败: "+err.Error(), "")
	}
	if err := antigravity.ClearToken(); err != nil {
		return env.fail(CodeFailure, "清空凭据失败: "+err.Error(), "")
	}
	if env.JSON {
		env.emitJSON(struct {
			payload
			Target string `json:"target"`
			Backup string `json:"auto_backup,omitempty"`
		}{payload{"logout", true, ""}, "antigravity", backup})
		return CodeOK
	}
	if backup != "" {
		fmt.Fprintf(env.Stdout, "已登出 agy(原账号已自动备份到槽位 %s)\n", backup)
	} else {
		fmt.Fprintln(env.Stdout, "已登出 agy(账号已保存在槽位)")
	}
	return CodeOK
}
