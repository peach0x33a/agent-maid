// Package gptapp 组合 ChatGPT 凭据层、账号库与官方 auth.json,
// 提供保存/导入/激活/重命名/登出等业务操作。TUI 与无头 CLI 共用这一套,
// 避免两条路径的行为分叉。
package gptapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-maid/internal/gptauth"
	"agent-maid/internal/gptsession"
	"agent-maid/internal/gptstore"
)

// Service ChatGPT 账号业务。
type Service struct {
	// Home 官方 Codex 数据目录;空串时按 CODEX_HOME / ~/.codex 解析。
	Home string
	// StorePath 账号库文件;空串时用 gptstore.DefaultPath()。
	StorePath string
}

// New 创建默认服务。
func New() *Service { return &Service{} }

func (s *Service) home() (string, error) { return gptauth.ResolveHome(s.Home) }

func (s *Service) storePath() string {
	if s.StorePath != "" {
		return s.StorePath
	}
	return gptstore.DefaultPath()
}

func (s *Service) loadStore() (*gptstore.Store, error) { return gptstore.Load(s.storePath()) }

// Current 读取官方 auth.json 里的当前凭据(未登录时返回 nil, nil)。
func (s *Service) Current() (*gptauth.Current, string, error) {
	home, err := s.home()
	if err != nil {
		return nil, "", err
	}
	current, err := gptauth.ReadCurrent(home)
	if err != nil {
		return nil, home, err
	}
	return current, home, nil
}

// List 返回账号库与当前生效的 account_id(官方 auth.json 实际生效的账号)。
func (s *Service) List() (accounts []*gptstore.Account, activeID string, liveAccountID string, err error) {
	st, err := s.loadStore()
	if err != nil {
		return nil, "", "", err
	}
	current, _, cerr := s.Current()
	if cerr != nil {
		return nil, "", "", cerr
	}
	if current != nil {
		liveAccountID = current.AccountID
	}
	return st.Data.Accounts, st.Data.ActiveID, liveAccountID, nil
}

// SaveResult 保存当前凭据的结果。
type SaveResult struct {
	Account *gptstore.Account
	Updated bool // 账号库里已有同一账号,本次是更新
}

// SaveCurrent 把官方 auth.json 里当前生效的凭据存入账号库。
// label 为空时用 email / account_id 自动命名;账号已存在且未显式命名时
// 保留用户改过的显示名。
func (s *Service) SaveCurrent(label string) (SaveResult, error) {
	home, err := s.home()
	if err != nil {
		return SaveResult{}, err
	}
	tokens, authMode, found, err := gptauth.ReadCurrentTokens(home)
	if err != nil {
		return SaveResult{}, err
	}
	if !found {
		if authMode != "" && authMode != gptauth.AuthModeChatGPT {
			return SaveResult{}, fmt.Errorf("当前是 %s 认证,没有可保存的 ChatGPT 凭据", PrettyAuthMode(authMode))
		}
		return SaveResult{}, fmt.Errorf("官方目录没有可保存的凭据(%s)", gptauth.AuthFilePath(home))
	}
	idPayload := gptsession.JWTPayload(tokens.IDToken)
	accessPayload := gptsession.JWTPayload(tokens.AccessToken)
	email, _ := idPayload["email"].(string)
	authClaim := gptsession.AuthClaimOf(accessPayload)
	if authClaim == nil {
		authClaim = gptsession.AuthClaimOf(idPayload)
	}
	planType, _ := authClaim["chatgpt_plan_type"].(string)
	userID, _ := authClaim["chatgpt_user_id"].(string)

	explicitLabel := strings.TrimSpace(label) != ""
	if !explicitLabel {
		label = firstNonEmpty(email, tokens.AccountID, "auth.json")
	}
	account := gptsession.Account{
		Label:        label,
		Email:        email,
		AccountID:    tokens.AccountID,
		UserID:       userID,
		PlanType:     planType,
		SourceFormat: "auth.json",
		SourceName:   filepath.Base(home),
		Refreshable:  tokens.RefreshToken != "",
		Tokens: gptsession.Tokens{
			IDToken:      tokens.IDToken,
			AccessToken:  tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
		},
	}
	if !account.Refreshable && accessPayload != nil {
		if exp, ok := accessPayload["exp"].(float64); ok && exp > 0 {
			t := time.Unix(int64(exp), 0).UTC()
			account.AccessExpiresAt = &t
		}
	}
	st, err := s.loadStore()
	if err != nil {
		return SaveResult{}, err
	}
	stored := gptstore.FromSession(account, time.Now().UTC())
	existing, idx := st.Find(stored.ID)
	existed := idx >= 0
	if existed && !explicitLabel {
		// 用户没指定名字时保留已有显示名,避免登出/保存时把用户改过的
		// 名字悄悄覆盖回邮箱
		stored.Label = existing.Label
	}
	st.Upsert(stored)
	if st.Data.ActiveID == "" {
		st.Data.ActiveID = stored.ID
	}
	if err := st.Save(); err != nil {
		return SaveResult{}, err
	}
	return SaveResult{Account: stored, Updated: existed}, nil
}

// Rename 改账号库里的显示名。
func (s *Service) Rename(target, label string) (*gptstore.Account, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, errors.New("显示名不能为空")
	}
	st, err := s.loadStore()
	if err != nil {
		return nil, err
	}
	acc, _ := st.Find(target)
	if acc == nil {
		return nil, fmt.Errorf("在账号库中找不到 %q", target)
	}
	acc.Label = label
	if err := st.Save(); err != nil {
		return nil, err
	}
	return acc, nil
}

// ActivateOptions 控制激活行为。
type ActivateOptions struct {
	// RequireSaved 为 true 时,官方 auth.json 里的当前凭据若不在账号库中,
	// 则拒绝激活。CLI 默认这么做:否则换过去之后,原来那个账号就只剩一份
	// 没人认得的 auth.json,再也切不回来。
	RequireSaved bool
}

// UnsavedCurrentError 当前登录的账号尚未收进账号库。
type UnsavedCurrentError struct {
	Account string // 当前账号的邮箱或 account_id,可能为空
}

func (e *UnsavedCurrentError) Error() string {
	who := "当前登录的账号"
	if e.Account != "" {
		who = "当前登录的账号 " + e.Account + " "
	}
	return who + "尚未保存到账号库"
}

// ActivateResult 激活结果。
type ActivateResult struct {
	Account *gptstore.Account
	Path    string // 写入的 auth.json 路径
}

// Activate 把账号库里的账号投影为官方 auth.json(保序合并、原子写入)。
func (s *Service) Activate(target string) (ActivateResult, error) {
	return s.ActivateWith(target, ActivateOptions{})
}

// ActivateWith 带选项的 Activate。
func (s *Service) ActivateWith(target string, opts ActivateOptions) (ActivateResult, error) {
	st, err := s.loadStore()
	if err != nil {
		return ActivateResult{}, err
	}
	acc, _ := st.Find(target)
	if acc == nil {
		return ActivateResult{}, fmt.Errorf("在账号库中找不到 %q", target)
	}
	home, err := s.home()
	if err != nil {
		return ActivateResult{}, err
	}
	if opts.RequireSaved {
		current, _, cerr := s.Current()
		if cerr != nil {
			return ActivateResult{}, cerr
		}
		// 当前 auth.json 里的凭据不在账号库 → 换过去就找不回来了
		if current != nil && !s.covered(st, current) {
			return ActivateResult{}, &UnsavedCurrentError{
				Account: firstNonEmpty(current.Email, current.AccountID),
			}
		}
	}
	existing, err := gptauth.ReadAuthFile(home)
	if err != nil {
		return ActivateResult{}, err
	}
	merged, err := gptauth.MergeAuthFile(existing, gptauth.Tokens{
		IDToken:      acc.Tokens.IDToken,
		AccessToken:  acc.Tokens.AccessToken,
		RefreshToken: acc.Tokens.RefreshToken,
		AccountID:    acc.AccountID,
	}, time.Now().UTC())
	if err != nil {
		return ActivateResult{}, err
	}
	if _, err := gptauth.WriteAuthFile(home, merged); err != nil {
		return ActivateResult{}, err
	}
	st.Data.ActiveID = acc.ID
	now := time.Now().UTC()
	acc.LastUsedAt = &now
	if err := st.Save(); err != nil {
		return ActivateResult{}, err
	}
	return ActivateResult{Account: acc, Path: gptauth.AuthFilePath(home)}, nil
}

// ImportResult 导入结果。
type ImportResult struct {
	Imported int
	Skipped  int
	BadDocs  int
	First    string
	Total    int // 找到的会话对象数
}

// Import 解析导入数据(文件内容或粘贴的 JSON)并入库。不自动激活。
// sourceName 仅用于记录来源。
func (s *Service) Import(data []byte, sourceName string) (ImportResult, error) {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	if len(data) == 0 {
		return ImportResult{}, errors.New("输入为空")
	}
	if sourceName == "" {
		sourceName = "pasted-json"
	}
	docs, badDocs, perr := ParseJSONDocs(data)
	if perr != nil {
		return ImportResult{}, perr
	}
	var records []gptsession.Record
	for i, doc := range docs {
		name := sourceName
		if len(docs) > 1 {
			name = fmt.Sprintf("%s#%d", sourceName, i+1)
		}
		records = append(records, gptsession.Collect(doc, name)...)
	}
	result := ImportResult{BadDocs: badDocs, Total: len(records)}
	if len(records) == 0 {
		return result, ErrNoAccounts
	}
	st, err := s.loadStore()
	if err != nil {
		return result, err
	}
	now := time.Now().UTC()
	for _, rec := range records {
		converted, err := gptsession.Convert(rec, now)
		if err != nil {
			result.Skipped++
			continue
		}
		account := gptstore.FromSession(converted, now)
		st.Upsert(account)
		if result.First == "" {
			result.First = account.Label
		}
		result.Imported++
	}
	if result.Imported == 0 {
		return result, ErrConvertFailed
	}
	if err := st.Save(); err != nil {
		return result, err
	}
	return result, nil
}

// ErrNoAccounts 输入里没有找到带 access_token 的会话对象。
var ErrNoAccounts = errors.New("未在输入中找到包含 access_token 的会话/账号对象(支持 WebSession 与 sub2api 导出)")

// ErrConvertFailed 找到了会话对象但全部无法转换。
var ErrConvertFailed = errors.New("全部会话对象均无法转换为账号")

// LogoutResult 登出结果。
type LogoutResult struct {
	Backup string // 未保存的当前账号被自动存入账号库时的显示名
}

// Logout 清空官方 auth.json(仅本地,不吊销服务端令牌)。当前凭据若不在
// 账号库里,先自动存入,避免登出即丢失。
func (s *Service) Logout() (LogoutResult, error) {
	st, err := s.loadStore()
	if err != nil {
		return LogoutResult{}, err
	}
	result := LogoutResult{}
	current, home, err := s.Current()
	if err != nil {
		return result, err
	}
	if current == nil {
		return result, errors.New("当前未登录,无需登出")
	}
	if !s.covered(st, current) {
		saved, serr := s.SaveCurrent("")
		if serr != nil {
			return result, fmt.Errorf("保存当前凭据失败: %w", serr)
		}
		result.Backup = saved.Account.Label
	}
	if err := gptauth.ClearAuthFile(home); err != nil {
		return result, fmt.Errorf("清空凭据失败: %w", err)
	}
	return result, nil
}

// covered 报告当前凭据是否已存在于账号库。
func (s *Service) covered(st *gptstore.Store, current *gptauth.Current) bool {
	if current == nil || current.AccountID == "" {
		return false
	}
	for _, a := range st.Data.Accounts {
		if a.AccountID == current.AccountID {
			return true
		}
	}
	return false
}

// Remove 把账号移入回收站(退出前可恢复)。
func (s *Service) Remove(target string) (string, error) {
	st, err := s.loadStore()
	if err != nil {
		return "", err
	}
	return st.DeleteToTrash(target)
}

// ReadImportFile 读取导入源:- 表示 stdin,否则按路径(支持 ~ 展开)。
func ReadImportFile(arg string, stdin io.Reader) (data []byte, sourceName string, err error) {
	if arg == "-" {
		b, rerr := io.ReadAll(stdin)
		if rerr != nil {
			return nil, "", rerr
		}
		return b, "stdin", nil
	}
	path := ExpandHome(arg)
	b, rerr := os.ReadFile(path)
	if rerr != nil {
		return nil, "", fmt.Errorf("读取文件失败: %w", rerr)
	}
	return b, filepath.Base(path), nil
}

// ExpandHome 展开 ~ 前缀。
func ExpandHome(p string) string {
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return home
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// PrettyAuthMode 认证模式的展示名(官方值为小写,展示遵循大小写)。
func PrettyAuthMode(mode string) string {
	switch mode {
	case "chatgpt":
		return "ChatGPT"
	case "apikey":
		return "API Key"
	}
	return mode
}

// ParseJSONDocs 解析导入输入,尽量宽容:
//  1. 连续 JSON 流:单对象/数组、CPA、sub2api 整包、拼接 JSON、以及
//     "空格分隔的多个 JSON"都算(从终端粘贴多行 JSONL 时,输入框会把
//     换行折叠成空格,所以这一条是粘贴场景的必经之路);
//  2. 流解析一个都拿不到时,退回逐行解析(JSONL 文件),跳过无效行并计数。
//
// 尾部有无法解析的残留时只计数不报错:导入是"能救多少救多少"。
func ParseJSONDocs(data []byte) (docs []any, badDocs int, err error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var v any
		derr := dec.Decode(&v)
		if derr != nil {
			if len(docs) > 0 {
				if !errors.Is(derr, io.EOF) {
					badDocs++ // 尾部残留
				}
				return docs, badDocs, nil
			}
			break
		}
		docs = append(docs, v)
	}

	// 流解析失败:按行重试(JSONL / 逐行拼接)。只接受对象与数组,
	// 单独的一行字符串/数字不是账号文档。
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var v any
		if json.Unmarshal([]byte(line), &v) != nil {
			badDocs++
			continue
		}
		switch v.(type) {
		case map[string]any, []any:
			docs = append(docs, v)
		default:
			badDocs++
		}
	}
	return docs, badDocs, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
