// Antigravity 账号槽位存储:每个槽位是 slots/ 下一个 JSON 文件,
// 快照当时的 oauth_creds.json 与 google_accounts.json 原文 + 邮箱标签。
package antigravity

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

	"agent-maid/internal/atomicio"
	"agent-maid/internal/meta"
)

// Slot 一个账号槽位。
type Slot struct {
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"` // 最近一次切换启用的时间
	Email      string     `json:"email"`                // 展示标签(来自 id_token/active)
	OAuthCreds Creds      `json:"oauthCreds"`
	Accounts   Accounts   `json:"googleAccounts"`
}

// Store 槽位存储,每槽位一个文件。
type Store struct{ Dir string }

// DefaultDir 返回默认槽位目录(agent-maid 统一配置下)。
func DefaultDir() string {
	if d := os.Getenv("ANTIGRAVITY_STORE_DIR"); d != "" {
		return d
	}
	return filepath.Join(meta.Dir(), "antigravity", "slots")
}

// NewStore 创建默认槽位存储。
func NewStore() *Store { return &Store{Dir: DefaultDir()} }

func (s *Store) slotsDir() string { return s.Dir }

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
	clean := strings.TrimSpace(nameRe.ReplaceAllString(name, "_"))
	if clean == "" || clean == "." || clean == ".." {
		return "", fmt.Errorf("槽位名 %q 无有效字符", name)
	}
	return clean, nil
}

func (s *Store) slotPath(name string) (string, error) {
	clean, err := SanitizeName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.slotsDir(), clean+".json"), nil
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
	path, err := s.slotPath(name)
	if err != nil {
		return Slot{}, err
	}
	return s.load(path)
}

// Has 判断槽位是否存在。
func (s *Store) Has(name string) bool {
	path, err := s.slotPath(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Save 创建或覆盖槽位。
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
	path := filepath.Join(s.slotsDir(), clean+".json")
	if err := atomicio.WriteFile(path, data, 0o600); err != nil {
		return Slot{}, err
	}
	return slot, nil
}

// TouchLastUsed 更新槽位的最后使用时间。
func (s *Store) TouchLastUsed(name string, t time.Time) error {
	slot, err := s.Get(name)
	if err != nil {
		return err
	}
	slot.LastUsedAt = &t
	_, err = s.Save(name, slot)
	return err
}

// Delete 删除槽位。
func (s *Store) Delete(name string) error {
	path, err := s.slotPath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// Rename 重命名槽位。
func (s *Store) Rename(oldName, newName string) error {
	slot, err := s.Get(oldName)
	if err != nil {
		return err
	}
	if _, err := s.Save(newName, slot); err != nil {
		return err
	}
	return s.Delete(oldName)
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
