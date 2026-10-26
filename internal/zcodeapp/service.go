// Package app 组合 zcode 凭据层与槽位存储,提供账号切换业务操作。
package zcodeapp

import (
	"errors"
	"fmt"
	"os"
	"time"

	"agent-maid/internal/zcode"
	"agent-maid/internal/zcodestore"
)

type Service struct {
	Store *zcodestore.Store
}

func New() *Service { return &Service{Store: zcodestore.New()} }

// CurrentInfo 当前账号状态。
type CurrentInfo struct {
	Exists  bool
	Meta    zcode.AccountMeta
	Creds   zcode.Credentials
	Decrypt bool // 解密是否成功(密钥指纹匹配)
}

// Current 读取并解密当前账号(仅内存)。
func (s *Service) Current() CurrentInfo {
	info := CurrentInfo{}
	creds, err := zcode.ReadCredentials()
	if err != nil {
		return info
	}
	info.Exists = true
	info.Creds = creds
	key := zcode.DeriveKey(zcode.CredentialSecret())
	info.Meta = creds.ExtractMeta(key)
	info.Decrypt = info.Meta.Provider != "" || info.Meta.UserID != ""
	return info
}

// SaveCurrent 把当前 credentials.json 保存进槽位。覆盖已存在槽位需 allowOverwrite。
func (s *Service) SaveCurrent(name string, allowOverwrite bool) (zcodestore.Slot, error) {
	creds, err := zcode.ReadCredentials()
	if err != nil {
		return zcodestore.Slot{}, fmt.Errorf("读取凭据失败: %w", err)
	}
	if !allowOverwrite && s.Store.Has(name) {
		return zcodestore.Slot{}, fmt.Errorf("槽位 %q 已存在", name)
	}
	key := zcode.DeriveKey(zcode.CredentialSecret())
	slot := zcodestore.Slot{
		KeyFingerprint: zcode.KeyFingerprint(key),
		Meta:           creds.ExtractMeta(key),
		Credentials:    creds,
	}
	saved, err := s.Store.Save(name, slot)
	if err != nil {
		return zcodestore.Slot{}, err
	}
	return saved, nil
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
	Account string // 当前账号的展示标签,可能为空
}

func (e *UnsavedCurrentError) Error() string {
	who := "当前账号"
	if e.Account != "" {
		who = "当前账号 " + e.Account + " "
	}
	return who + "尚未保存到槽位"
}

// Apply 将槽位内容写回 credentials.json。
// 若当前凭据未被任何槽位覆盖,先自动备份到 auto-<时间戳> 槽位,避免数据丢失。
// 返回自动备份的槽位名(未发生则为空)。
func (s *Service) Apply(name string) (string, error) {
	return s.ApplyWith(name, ApplyOptions{})
}

// ApplyWith 带选项的 Apply。
func (s *Service) ApplyWith(name string, opts ApplyOptions) (autoBackup string, err error) {
	slot, err := s.Store.Get(name)
	if err != nil {
		return "", fmt.Errorf("读取槽位 %q 失败: %w", name, err)
	}
	if len(slot.Credentials) == 0 {
		return "", fmt.Errorf("槽位 %q 没有凭据内容", name)
	}
	current, err := zcode.ReadCredentials()
	switch {
	case err == nil:
		if zcode.EqualCredentials(current, slot.Credentials) {
			_ = s.Store.TouchLastUsed(name, time.Now()) // 确认为当前账号
			return "", nil
		}
		if !s.currentCovered(current) {
			if opts.RequireSaved {
				return "", &UnsavedCurrentError{Account: describeAccount(current.ExtractMeta(zcode.DeriveKey(zcode.CredentialSecret())))}
			}
			backupName := fmt.Sprintf("auto-%s", time.Now().Format("0102-150405"))
			key := zcode.DeriveKey(zcode.CredentialSecret())
			backup := zcodestore.Slot{
				KeyFingerprint: zcode.KeyFingerprint(key),
				Meta:           current.ExtractMeta(key),
				Credentials:    current,
			}
			if _, err := s.Store.Save(backupName, backup); err != nil {
				return "", fmt.Errorf("自动备份当前账号失败: %w", err)
			}
			autoBackup = backupName
		}
	case errors.Is(err, os.ErrNotExist):
		// 凭据不存在,直接写入即可
	default:
		return "", fmt.Errorf("读取当前凭据失败: %w", err)
	}
	if err := zcode.WriteCredentials(slot.Credentials); err != nil {
		return autoBackup, fmt.Errorf("写入凭据失败: %w", err)
	}
	_ = s.Store.TouchLastUsed(name, time.Now())
	return autoBackup, nil
}

// CurrentCovered 报告当前凭据是否已被某个槽位覆盖(已保存)。
func (s *Service) CurrentCovered() bool {
	current, err := zcode.ReadCredentials()
	return err == nil && s.currentCovered(current)
}

// BackupCurrentIfUncovered 当前凭据未被任何槽位覆盖时,自动备份到
// auto-<时间戳> 槽位。返回备份槽位名(未备份为空)。
func (s *Service) BackupCurrentIfUncovered() (string, error) {
	current, err := zcode.ReadCredentials()
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
	key := zcode.DeriveKey(zcode.CredentialSecret())
	if _, err := s.Store.Save(name, zcodestore.Slot{
		KeyFingerprint: zcode.KeyFingerprint(key),
		Meta:           current.ExtractMeta(key),
		Credentials:    current,
	}); err != nil {
		return "", err
	}
	return name, nil
}

// currentCovered 判断当前凭据是否与某个槽位完全一致(即不会因切换而丢失)。
func (s *Service) currentCovered(current zcode.Credentials) bool {
	slots, err := s.Store.List()
	if err != nil {
		return false
	}
	for _, slot := range slots {
		if zcode.EqualCredentials(current, slot.Credentials) {
			return true
		}
	}
	return false
}

// describeAccount 把账号摘要拼成展示标签(provider / 用户名)。
func describeAccount(meta zcode.AccountMeta) string {
	parts := make([]string, 0, 2)
	if meta.Provider != "" {
		parts = append(parts, meta.Provider)
	}
	name := meta.DisplayName
	if name == "" {
		name = meta.Username
	}
	if name != "" {
		parts = append(parts, name)
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " / "
		}
		out += p
	}
	return out
}
