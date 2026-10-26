// 业务操作:读取当前登录账号、保存槽位、切换(带未覆盖自动备份)。
package antigravity

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Service 组合凭据层与槽位存储。
type Service struct {
	Store *Store
}

// New 创建服务。
func New() *Service { return &Service{Store: NewStore()} }

// CurrentInfo 当前登录状态。
type CurrentInfo struct {
	Exists    bool   // antigravity-oauth-token 是否存在
	Email     string // 账号邮箱
	AuthMode  string // consumer / enterprise
	Token     Token  // 凭据原文
	State     TokenState
	LoggedOut string // 最近一次登出留下的备份文件名(未登录时提示用)
}

// Current 读取当前登录账号(纯本地,不联网)。
func (s *Service) Current() CurrentInfo {
	info := CurrentInfo{}
	tok, err := ReadToken()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			info.LoggedOut = latestLoggedOut()
		}
		return info
	}
	info.Exists = true
	info.Token = tok
	info.Email = TokenEmail(tok)
	info.AuthMode = TokenAuthMethod(tok)
	info.State = TokenStateOf(tok)
	return info
}

// SaveCurrent 把当前登录凭据保存进槽位。覆盖已存在槽位需 allowOverwrite。
func (s *Service) SaveCurrent(name string, allowOverwrite bool) (Slot, error) {
	tok, err := ReadToken()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Slot{}, fmt.Errorf("没有找到 %s,请先在 agy 里登录", TokenPath())
		}
		return Slot{}, fmt.Errorf("读取凭据失败: %w", err)
	}
	if !allowOverwrite && s.Store.Has(name) {
		return Slot{}, fmt.Errorf("槽位 %q 已存在", name)
	}
	slot, err := s.Store.Save(name, Slot{
		Email: TokenEmail(tok),
		Token: tok,
	})
	if err != nil {
		return Slot{}, err
	}
	slot.State = TokenStateOf(slot.Token)
	return slot, nil
}

// ApplyOptions 控制切换行为。
type ApplyOptions struct {
	// RequireSaved 为 true 时,当前凭据未被任何槽位覆盖则拒绝切换,
	// 而不是自动备份。CLI 默认这么做:auto-<时间戳> 槽位事后没人认得出
	// 是谁,让调用方先显式 save 更可靠。
	RequireSaved bool
}

// UnsavedCurrentError 当前账号尚未保存到槽位,切换会造成"没人认得"的备份。
type UnsavedCurrentError struct {
	Account string // 当前账号的邮箱,可能为空
}

func (e *UnsavedCurrentError) Error() string {
	who := "当前账号"
	if e.Account != "" {
		who = "当前账号 " + e.Account + " "
	}
	return who + "尚未保存到槽位"
}

// Apply 将槽位凭据写回 agy 的凭据文件。若当前凭据未被任何槽位覆盖,
// 先自动备份到 auto-<时间戳> 槽位。返回自动备份的槽位名(未发生则为空)。
func (s *Service) Apply(name string) (string, error) {
	return s.ApplyWith(name, ApplyOptions{})
}

// ApplyWith 带选项的 Apply。
func (s *Service) ApplyWith(name string, opts ApplyOptions) (autoBackup string, err error) {
	slot, err := s.Store.Get(name)
	if err != nil {
		return "", fmt.Errorf("读取槽位 %q 失败: %w", name, err)
	}
	if len(slot.Token) == 0 {
		return "", fmt.Errorf("槽位 %q 没有凭据内容", name)
	}
	current, err := ReadToken()
	switch {
	case err == nil:
		if EqualToken(current, slot.Token) {
			_ = s.Store.TouchLastUsed(name, time.Now()) // 确认为当前账号
			return "", nil
		}
		if !s.currentCovered(current) {
			if opts.RequireSaved {
				return "", &UnsavedCurrentError{Account: TokenEmail(current)}
			}
			backupName := fmt.Sprintf("auto-%s", time.Now().Format("0102-150405"))
			if _, err := s.Store.Save(backupName, Slot{
				Email: TokenEmail(current),
				Token: current,
			}); err != nil {
				return "", fmt.Errorf("自动备份当前账号失败: %w", err)
			}
			autoBackup = backupName
		}
	case errors.Is(err, os.ErrNotExist):
		// 未登录,直接写入即可
	default:
		return "", fmt.Errorf("读取当前凭据失败: %w", err)
	}
	if err := WriteToken(slot.Token); err != nil {
		return autoBackup, fmt.Errorf("写入凭据失败: %w", err)
	}
	_ = s.Store.TouchLastUsed(name, time.Now())
	return autoBackup, nil
}

// CurrentCovered 报告当前凭据是否已被某个槽位覆盖(已保存)。
func (s *Service) CurrentCovered() bool {
	current, err := ReadToken()
	return err == nil && s.currentCovered(current)
}

// BackupCurrentIfUncovered 当前凭据未被任何槽位覆盖时,自动备份到
// auto-<时间戳> 槽位。返回备份槽位名(未备份为空)。
func (s *Service) BackupCurrentIfUncovered() (string, error) {
	current, err := ReadToken()
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if s.currentCovered(current) {
		return "", nil
	}
	name := fmt.Sprintf("auto-%s", time.Now().Format("0102-150405"))
	if _, err := s.Store.Save(name, Slot{
		Email: TokenEmail(current),
		Token: current,
	}); err != nil {
		return "", err
	}
	return name, nil
}

// currentCovered 判断当前凭据是否与某个槽位完全一致(即不会因切换而丢失)。
func (s *Service) currentCovered(current Token) bool {
	slots, err := s.Store.List()
	if err != nil {
		return false
	}
	for _, slot := range slots {
		if EqualToken(current, slot.Token) {
			return true
		}
	}
	return false
}

// latestLoggedOut 返回最近一次登出留下的备份文件名(不含目录)。
// 按修改时间取最新:文件名的后缀格式在 agy 各版本间并不统一
// (有 Y<MMDD>T<HHMMSS>Z,也有纯数字),不能靠字典序比较。
func latestLoggedOut() string {
	entries, err := os.ReadDir(CLIDir())
	if err != nil {
		return ""
	}
	prefix := "." + tokenFileName + ".logged-out-"
	latest := ""
	var latestAt time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if latest == "" || info.ModTime().After(latestAt) {
			latest, latestAt = e.Name(), info.ModTime()
		}
	}
	return latest
}
