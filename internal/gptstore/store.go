// Package store keeps the imported accounts on disk so the tool can switch
// between them later without re-importing — the same "credential pool →
// project into the official directory" model as Cockpit Tools.
package gptstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agent-maid/internal/atomicio"
	"agent-maid/internal/gptsession"
	"agent-maid/internal/meta"
)

// Account is the persisted form of a converted session.
type Account struct {
	ID               string            `json:"id"`
	Label            string            `json:"label"`
	Email            string            `json:"email,omitempty"`
	AccountID        string            `json:"account_id,omitempty"`
	UserID           string            `json:"chatgpt_user_id,omitempty"`
	PlanType         string            `json:"plan_type,omitempty"`
	WorkspaceID      string            `json:"workspace_id,omitempty"`
	SourceFormat     string            `json:"source_format"`
	SourceName       string            `json:"source_name,omitempty"`
	ImportedAt       time.Time         `json:"imported_at"`
	AccessExpiresAt  *time.Time        `json:"access_expires_at,omitempty"`
	Refreshable      bool              `json:"refreshable"`
	LastUsedAt       *time.Time        `json:"last_used_at,omitempty"` // 最近一次激活时间
	IDTokenSynthetic bool              `json:"id_token_synthetic,omitempty"`
	IDTokenReplaced  bool              `json:"id_token_replaced,omitempty"`
	Tokens           gptsession.Tokens `json:"tokens"`
}

type Data struct {
	Version  int        `json:"version"`
	ActiveID string     `json:"active_id,omitempty"`
	Accounts []*Account `json:"accounts"`
}

type Store struct {
	Path string
	Data *Data
}

// DefaultPath is the unified agent-maid location for the ChatGPT account
// library (<config>/chatgpt/store.json).
func DefaultPath() string { return meta.GPTStorePath() }

// Load reads the store; a missing file is an empty store, but a corrupt one
// is an error so we never silently clobber saved credentials.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Store{Path: path, Data: &Data{Version: 1, Accounts: []*Account{}}}, nil
	}
	if err != nil {
		return nil, err
	}
	var parsed Data
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("账号库 %s 无法解析: %w", path, err)
	}
	if parsed.Version == 0 {
		parsed.Version = 1
	}
	if parsed.Accounts == nil {
		parsed.Accounts = []*Account{}
	}
	return &Store{Path: path, Data: &parsed}, nil
}

func (s *Store) Save() error {
	raw, err := json.MarshalIndent(s.Data, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return atomicio.WriteFile(s.Path, raw, 0o600)
}

// Upsert replaces the stored entry with the same ID, or appends.
func (s *Store) Upsert(account *Account) {
	for i, existing := range s.Data.Accounts {
		if existing.ID == account.ID {
			s.Data.Accounts[i] = account
			return
		}
	}
	s.Data.Accounts = append(s.Data.Accounts, account)
}

// Find resolves a user-supplied target: 1-based index, exact id / account_id /
// email / label, or a unique case-insensitive prefix of any of those.
func (s *Store) Find(target string) (*Account, int) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" || len(s.Data.Accounts) == 0 {
		return nil, -1
	}
	if index, err := strconv.Atoi(trimmed); err == nil && index >= 1 && index <= len(s.Data.Accounts) {
		return s.Data.Accounts[index-1], index - 1
	}
	needle := strings.ToLower(trimmed)
	for i, account := range s.Data.Accounts {
		for _, field := range []string{account.ID, account.AccountID, account.Email, account.Label} {
			if field != "" && strings.ToLower(field) == needle {
				return account, i
			}
		}
	}
	hits := -1
	for i, account := range s.Data.Accounts {
		for _, field := range []string{account.ID, account.AccountID, account.Email, account.Label} {
			if field != "" && strings.HasPrefix(strings.ToLower(field), needle) {
				if hits >= 0 {
					return nil, -1 // ambiguous prefix
				}
				hits = i
				break
			}
		}
	}
	if hits < 0 {
		return nil, -1
	}
	return s.Data.Accounts[hits], hits
}

// trashDir 返回账号回收站目录。
func (s *Store) trashDir() string {
	return filepath.Join(filepath.Dir(s.Path), "trash")
}

// DeleteToTrash 把账号移入回收站(从账号库移除但保留副本),返回回收站文件名。
func (s *Store) DeleteToTrash(target string) (string, error) {
	acc, ok := s.Remove(target)
	if !ok {
		return "", fmt.Errorf("在账号库中找不到 %q", target)
	}
	dir := s.trashDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(acc, "", "  ")
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%d-account.json", time.Now().UnixNano())
	if err := atomicio.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		return "", err
	}
	if err := s.Save(); err != nil {
		return "", err
	}
	return name, nil
}

// RestoreLatestFromTrash 从回收站恢复最近删除的账号,返回显示名。
func (s *Store) RestoreLatestFromTrash() (string, error) {
	entries, err := os.ReadDir(s.trashDir())
	if err != nil {
		return "", errors.New("回收站为空")
	}
	var latest string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") && e.Name() > latest {
			latest = e.Name()
		}
	}
	if latest == "" {
		return "", errors.New("回收站为空")
	}
	data, err := os.ReadFile(filepath.Join(s.trashDir(), latest))
	if err != nil {
		return "", err
	}
	var acc Account
	if err := json.Unmarshal(data, &acc); err != nil {
		return "", fmt.Errorf("回收站文件损坏: %w", err)
	}
	s.Upsert(&acc)
	if err := s.Save(); err != nil {
		return "", err
	}
	if err := os.Remove(filepath.Join(s.trashDir(), latest)); err != nil {
		return "", err
	}
	return acc.Label, nil
}

// PurgeTrash 清空回收站(正式删除)。
func (s *Store) PurgeTrash() error {
	return os.RemoveAll(s.trashDir())
}

func (s *Store) Remove(target string) (*Account, bool) {
	account, index := s.Find(target)
	if account == nil {
		return nil, false
	}
	s.Data.Accounts = append(s.Data.Accounts[:index], s.Data.Accounts[index+1:]...)
	if s.Data.ActiveID == account.ID {
		s.Data.ActiveID = ""
	}
	return account, true
}

func (s *Store) Active() *Account {
	if s.Data.ActiveID == "" {
		return nil
	}
	for _, account := range s.Data.Accounts {
		if account.ID == s.Data.ActiveID {
			return account
		}
	}
	return nil
}

// FromSession derives the persisted account from a converted session. The ID
// keys on the ChatGPT account id (stable across token refreshes) and falls
// back to email, then to a hash of the access token.
func FromSession(a gptsession.Account, now time.Time) *Account {
	id := strings.ToLower(firstNonEmpty(a.AccountID, a.Email))
	if id == "" {
		id = "token-" + shortHash(a.Tokens.AccessToken)
	}
	return &Account{
		ID:               id,
		Label:            a.Label,
		Email:            a.Email,
		AccountID:        a.AccountID,
		UserID:           a.UserID,
		PlanType:         a.PlanType,
		WorkspaceID:      a.WorkspaceID,
		SourceFormat:     a.SourceFormat,
		SourceName:       a.SourceName,
		ImportedAt:       now,
		AccessExpiresAt:  a.AccessExpiresAt,
		Refreshable:      a.Refreshable,
		IDTokenSynthetic: a.IDTokenSynthetic,
		IDTokenReplaced:  a.IDTokenReplaced,
		Tokens:           a.Tokens,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}
