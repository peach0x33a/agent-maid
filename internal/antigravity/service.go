// 业务操作:读取当前账号、保存槽位、切换(带未覆盖自动备份)。
package antigravity

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// Service 组合凭据层与槽位存储。
type Service struct {
	Store *Store
}

// New 创建服务。
func New() *Service { return &Service{Store: NewStore()} }

// CurrentInfo 当前凭据状态。
type CurrentInfo struct {
	Exists   bool // oauth_creds.json 是否存在
	Email    string
	Active   string
	Creds    Creds
	Accounts Accounts
}

// Current 读取当前账号(纯本地,不联网)。
func (s *Service) Current() CurrentInfo {
	info := CurrentInfo{}
	creds, err := ReadCreds()
	if err != nil {
		return info
	}
	info.Exists = true
	info.Creds = creds
	if accounts, err := ReadAccounts(); err == nil {
		info.Accounts = accounts
		info.Active = accounts.ActiveAccount()
	}
	info.Email = emailFromIDToken(str(creds["id_token"]))
	if info.Email == "" {
		info.Email = info.Active
	}
	return info
}

// SaveCurrent 把当前凭据保存进槽位。覆盖已存在槽位需 allowOverwrite。
func (s *Service) SaveCurrent(name string, allowOverwrite bool) (Slot, error) {
	creds, err := ReadCreds()
	if err != nil {
		return Slot{}, fmt.Errorf("读取凭据失败: %w", err)
	}
	if !allowOverwrite && s.Store.Has(name) {
		return Slot{}, fmt.Errorf("槽位 %q 已存在", name)
	}
	accounts, _ := ReadAccounts()
	slot := Slot{
		Email:      firstNonEmpty(emailFromIDToken(str(creds["id_token"])), accounts.ActiveAccount(), "unknown"),
		OAuthCreds: creds,
		Accounts:   accounts,
	}
	return s.Store.Save(name, slot)
}

// Apply 将槽位写回凭据文件。若当前凭据未被任何槽位覆盖,
// 先自动备份到 auto-<时间戳> 槽位。返回自动备份的槽位名(未发生则为空)。
func (s *Service) Apply(name string) (autoBackup string, err error) {
	slot, err := s.Store.Get(name)
	if err != nil {
		return "", fmt.Errorf("读取槽位 %q 失败: %w", name, err)
	}
	if slot.OAuthCreds == nil {
		return "", fmt.Errorf("槽位 %q 没有凭据内容", name)
	}
	current, err := ReadCreds()
	switch {
	case err == nil:
		if EqualCreds(current, slot.OAuthCreds) && EqualAccounts(mustReadAccounts(), slot.Accounts) {
			return "", nil // 内容一致,无需切换
		}
		if !s.currentCovered(current) {
			backupName := fmt.Sprintf("auto-%s", time.Now().Format("0102-150405"))
			accounts, _ := ReadAccounts()
			if _, err := s.Store.Save(backupName, Slot{
				Email:      firstNonEmpty(emailFromIDToken(str(current["id_token"])), accounts.ActiveAccount(), "unknown"),
				OAuthCreds: current,
				Accounts:   accounts,
			}); err != nil {
				return "", fmt.Errorf("自动备份当前账号失败: %w", err)
			}
			autoBackup = backupName
		}
	case errors.Is(err, os.ErrNotExist):
		// 凭据不存在,直接写入即可
	default:
		return "", fmt.Errorf("读取当前凭据失败: %w", err)
	}
	if err := WriteCreds(slot.OAuthCreds); err != nil {
		return autoBackup, fmt.Errorf("写入凭据失败: %w", err)
	}
	if slot.Accounts != nil {
		if err := WriteAccounts(slot.Accounts); err != nil {
			return autoBackup, fmt.Errorf("写入账号文件失败: %w", err)
		}
	}
	return autoBackup, nil
}

// CurrentCovered 报告当前凭据是否已被某个槽位覆盖(已保存)。
func (s *Service) CurrentCovered() bool {
	current, err := ReadCreds()
	return err == nil && s.currentCovered(current)
}

// BackupCurrentIfUncovered 当前凭据未被任何槽位覆盖时,自动备份到
// auto-<时间戳> 槽位。返回备份槽位名(未备份为空)。
func (s *Service) BackupCurrentIfUncovered() (string, error) {
	current, err := ReadCreds()
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
	accounts, _ := ReadAccounts()
	if _, err := s.Store.Save(name, Slot{
		Email:      firstNonEmpty(emailFromIDToken(str(current["id_token"])), accounts.ActiveAccount(), "unknown"),
		OAuthCreds: current,
		Accounts:   accounts,
	}); err != nil {
		return "", err
	}
	return name, nil
}

// currentCovered 判断当前凭据是否与某个槽位完全一致。
func (s *Service) currentCovered(current Creds) bool {
	slots, err := s.Store.List()
	if err != nil {
		return false
	}
	for _, slot := range slots {
		if EqualCreds(current, slot.OAuthCreds) {
			return true
		}
	}
	return false
}

func mustReadAccounts() Accounts {
	accounts, _ := ReadAccounts()
	return accounts
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
