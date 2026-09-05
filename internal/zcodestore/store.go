// Package store 负责账号槽位的持久化。
//
// 每个槽位是 <agent-maid 配置目录>/zcode/slots/ 下的一个 JSON 文件,
// 内容为当时的 credentials.json 原文 + 解密出的账号摘要 + 密钥指纹。
// 槽位文件权限 0600(与凭据文件同级保护)。
package zcodestore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"agent-maid/internal/meta"
	"agent-maid/internal/zcode"
)

// Slot 一个账号槽位。
type Slot struct {
	Name           string            `json:"name"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
	KeyFingerprint string            `json:"keyFingerprint"` // 保存时所用密钥指纹
	Meta           zcode.AccountMeta `json:"meta"`
	Credentials    zcode.Credentials `json:"credentials"`
}

// Store 槽位存储,基于目录下每槽位一个文件。
type Store struct{ Dir string }

// DefaultDir 返回默认存储目录。ZCODE_ACC_SWITCH_DIR 优先(沿用旧工具的习惯)。
func DefaultDir() string {
	if d := os.Getenv("ZCODE_ACC_SWITCH_DIR"); d != "" {
		return d
	}
	return meta.ZcodeBaseDir()
}

func New() *Store { return &Store{Dir: DefaultDir()} }

func (s *Store) slotsDir() string { return filepath.Join(s.Dir, "slots") }

var nameRe = regexp.MustCompile(`[^a-zA-Z0-9\p{Han}_\- ]`)

// SanitizeName 清洗槽位名(防路径穿越等)。
func SanitizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("槽位名不能为空")
	}
	if len(name) > 64 {
		name = name[:64]
	}
	clean := nameRe.ReplaceAllString(name, "_")
	clean = strings.TrimSpace(clean)
	if clean == "" || clean == "." || clean == ".." {
		return "", fmt.Errorf("槽位名 %q 无有效字符", name)
	}
	return clean, nil
}

func (s *Store) slotPath(name string) string {
	return filepath.Join(s.slotsDir(), name+".json")
}

// List 返回全部槽位,按名字排序。
func (s *Store) List() ([]Slot, error) {
	entries, err := os.ReadDir(s.slotsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var slots []Slot
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		slot, err := s.load(filepath.Join(s.slotsDir(), e.Name()))
		if err != nil {
			continue // 单个坏文件不影响整体
		}
		slots = append(slots, slot)
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].Name < slots[j].Name })
	return slots, nil
}

// Get 按名字取槽位。
func (s *Store) Get(name string) (Slot, error) {
	path, err := s.resolve(name)
	if err != nil {
		return Slot{}, err
	}
	return s.load(path)
}

// Has 判断槽位是否存在。
func (s *Store) Has(name string) bool {
	path, err := s.resolve(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Save 创建或覆盖槽位,返回落盘后的槽位(名字已清洗、时间已更新)。
func (s *Store) Save(name string, slot Slot) (Slot, error) {
	clean, err := SanitizeName(name)
	if err != nil {
		return Slot{}, err
	}
	if err := os.MkdirAll(s.slotsDir(), 0o700); err != nil {
		return Slot{}, err
	}
	slot.Name = clean
	now := time.Now()
	slot.UpdatedAt = now
	if slot.CreatedAt.IsZero() {
		slot.CreatedAt = now
	}
	data, err := json.MarshalIndent(slot, "", "  ")
	if err != nil {
		return Slot{}, err
	}
	// 原子写入:先写临时文件再 rename
	tmp, err := os.CreateTemp(s.slotsDir(), ".slot-tmp-*")
	if err != nil {
		return Slot{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return Slot{}, err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return Slot{}, err
	}
	if err := tmp.Close(); err != nil {
		return Slot{}, err
	}
	if err := os.Rename(tmpName, s.slotPath(clean)); err != nil {
		return Slot{}, err
	}
	return slot, nil
}

// Delete 删除槽位。
func (s *Store) Delete(name string) error {
	path, err := s.resolve(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return nil
}

// Rename 重命名槽位。
func (s *Store) Rename(oldName, newName string) error {
	cleanOld, err := SanitizeName(oldName)
	if err != nil {
		return err
	}
	cleanNew, err := SanitizeName(newName)
	if err != nil {
		return err
	}
	slot, err := s.Get(cleanOld)
	if err != nil {
		return err
	}
	slot.Name = cleanNew
	if _, err := s.Save(cleanNew, slot); err != nil {
		return err
	}
	return os.Remove(s.slotPath(cleanOld))
}

func (s *Store) resolve(name string) (string, error) {
	clean, err := SanitizeName(name)
	if err != nil {
		return "", err
	}
	return s.slotPath(clean), nil
}

func (s *Store) load(path string) (Slot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Slot{}, err
	}
	var slot Slot
	if err := json.Unmarshal(data, &slot); err != nil {
		return Slot{}, fmt.Errorf("%s 解析失败: %w", filepath.Base(path), err)
	}
	return slot, nil
}
