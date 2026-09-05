package gptsession

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	SourceWebSession = "websession"
	SourceSub2API    = "sub2api"
)

const syntheticIDTokenSignature = "synthetic"

// Fallback lifetime for the synthetic id_token when neither the access token
// nor the input carries an expiry (matches the reference converter).
const syntheticTokenLifetime = 90 * 24 * time.Hour

// Tokens is the credential triple plus the WebSession-only session token.
type Tokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	SessionToken string `json:"session_token,omitempty"`
}

// Account is the normalized form every supported input format converts into.
type Account struct {
	Label            string
	Email            string
	AccountID        string
	UserID           string
	PlanType         string
	WorkspaceID      string
	SourceFormat     string
	SourceName       string
	SourcePath       string
	Refreshable      bool
	AccessExpiresAt  *time.Time
	IDTokenSynthetic bool
	IDTokenReplaced  bool
	Tokens           Tokens
	Warnings         []string
}

// Convert normalizes one collected record into an Account, following the
// field-resolution chains of the reference converter (GPTSession2CPAandSub2API).
// When the record carries no real id_token (typical for WebSession), a Codex-
// parseable placeholder JWT is synthesized; a real id_token whose account
// claim contradicts the resolved account is replaced the same way, because
// the "new access_token + old ChatGPT-Account-Id" pairing makes the official
// client demand a re-login.
func Convert(record Record, now time.Time) (Account, error) {
	rec := record.Value
	tokens := obj(rec["tokens"])
	token := obj(rec["token"])
	creds := obj(rec["credentials"])
	user := obj(rec["user"])
	account := obj(rec["account"])
	meta := obj(rec["meta"])
	psd := obj(rec["providerSpecificData"])

	accessToken := firstNonEmpty(
		rec["accessToken"], rec["access_token"],
		tokens["accessToken"], tokens["access_token"],
		token["accessToken"], token["access_token"],
		creds["accessToken"], creds["access_token"],
	)
	if accessToken == "" {
		return Account{}, fmt.Errorf("缺少 accessToken")
	}
	sessionToken := firstNonEmpty(
		rec["sessionToken"], rec["session_token"],
		tokens["sessionToken"], tokens["session_token"],
		token["sessionToken"], token["session_token"],
		creds["sessionToken"], creds["session_token"],
	)
	refreshToken := firstNonEmpty(
		rec["refreshToken"], rec["refresh_token"],
		tokens["refreshToken"], tokens["refresh_token"],
		token["refreshToken"], token["refresh_token"],
		creds["refreshToken"], creds["refresh_token"],
	)
	inputIDToken := firstNonEmpty(
		rec["idToken"], rec["id_token"],
		tokens["idToken"], tokens["id_token"],
		token["idToken"], token["id_token"],
		creds["idToken"], creds["id_token"],
	)

	payload := JWTPayload(accessToken)
	idPayload := JWTPayload(inputIDToken)
	auth := openAIAuthClaimOf(payload)
	idAuth := openAIAuthClaimOf(idPayload)
	profile := openAIProfileClaimOf(payload)

	refreshable := refreshToken != ""

	var accessExpiresAt *time.Time
	if !refreshable {
		if exp := unixSeconds(payload["exp"]); exp > 0 {
			t := time.Unix(exp, 0).UTC()
			accessExpiresAt = &t
		} else if t, ok := normalizeTimestamp(firstNonEmpty(
			rec["expires"], rec["expiresAt"], rec["expired"], rec["expires_at"], creds["expires_at"],
		)); ok {
			accessExpiresAt = &t
		}
	}

	email := firstNonEmpty(
		user["email"], rec["email"],
		meta["label"], rec["label"],
		creds["email"], psd["email"],
		profile["email"], idPayload["email"], payload["email"],
	)

	recordAccountID := firstNonEmpty(
		account["id"], rec["account_id"],
		tokens["accountId"], tokens["account_id"],
		rec["chatgptAccountId"], rec["chatgpt_account_id"],
		meta["chatgptAccountId"], meta["chatgpt_account_id"],
		tokens["chatgptAccountId"], tokens["chatgpt_account_id"],
		psd["chatgptAccountId"], psd["chatgpt_account_id"],
		creds["account_id"], creds["chatgpt_account_id"],
	)
	if recordAccountID == "" && firstNonEmpty(rec["provider"]) == "codex" {
		// 9router-style entries keep the account id in `id`.
		recordAccountID = firstNonEmpty(rec["id"])
	}
	tokenAccountID := firstNonEmpty(auth["chatgpt_account_id"])
	idTokenAccountID := firstNonEmpty(idAuth["chatgpt_account_id"])

	var warnings []string
	accountID := recordAccountID
	switch {
	case tokenAccountID != "" && recordAccountID != "" && tokenAccountID != recordAccountID:
		// The access token is the credential the API actually validates
		// against, so its claim wins over stale record metadata.
		accountID = tokenAccountID
		warnings = append(warnings, fmt.Sprintf(
			"输入记录携带的 account_id（%s）与 access_token 声明（%s）不一致，已按 access_token 为准",
			recordAccountID, tokenAccountID))
	case accountID == "":
		accountID = tokenAccountID
	}
	if accountID == "" {
		accountID = idTokenAccountID
	}

	userID := firstNonEmpty(
		user["id"], rec["user_id"], rec["chatgptUserId"],
		psd["chatgptUserId"], psd["chatgpt_user_id"],
		creds["chatgptUserId"], creds["chatgpt_user_id"],
		auth["chatgpt_user_id"], auth["user_id"],
		idAuth["chatgpt_user_id"], idAuth["user_id"],
	)
	planType := firstNonEmpty(
		account["planType"], account["plan_type"],
		rec["planType"], rec["plan_type"],
		psd["chatgptPlanType"], psd["chatgpt_plan_type"],
		creds["plan_type"],
		auth["chatgpt_plan_type"], idAuth["chatgpt_plan_type"],
	)
	workspaceID := firstNonEmpty(
		account["workspaceId"], account["workspace_id"],
		rec["workspaceId"], rec["workspace_id"],
		meta["workspaceId"], meta["workspace_id"],
		psd["workspaceId"], psd["workspace_id"],
		creds["workspace_id"],
		payload["workspace_id"], idPayload["workspace_id"],
	)

	idToken := inputIDToken
	synthetic := false
	replaced := false
	syntheticExp := now.Add(syntheticTokenLifetime).Unix()
	if accessExpiresAt != nil {
		syntheticExp = accessExpiresAt.Unix()
	}
	switch {
	case idToken == "":
		if accountID != "" {
			idToken = buildSyntheticIDToken(email, accountID, planType, userID, syntheticExp, now)
			synthetic = true
		}
	case idTokenAccountID != "" && accountID != "" && idTokenAccountID != accountID:
		idToken = buildSyntheticIDToken(email, accountID, planType, userID, syntheticExp, now)
		synthetic = true
		replaced = true
		warnings = append(warnings, "id_token 的账号声明与 account_id 不一致（疑似新 access_token 配旧 id_token），已替换为可解析的占位 id_token，避免官方端要求重新登录")
	}

	if !refreshable {
		warnings = append(warnings, "缺少 refresh_token：access_token 过期后官方端无法自动续期，需要重新导入")
	}
	if accountID == "" {
		warnings = append(warnings, "无法确定 chatgpt account_id，官方 Codex 可能拒绝该凭据")
	}

	sourceName := record.SourceName
	if sourceName == "" {
		sourceName = "pasted-json"
	}
	label := firstNonEmptyString(email, sourceName, "ChatGPT Account")

	return Account{
		Label:            label,
		Email:            email,
		AccountID:        accountID,
		UserID:           userID,
		PlanType:         planType,
		WorkspaceID:      workspaceID,
		SourceFormat:     sourceFormatOf(rec),
		SourceName:       sourceName,
		SourcePath:       record.Path,
		Refreshable:      refreshable,
		AccessExpiresAt:  accessExpiresAt,
		IDTokenSynthetic: synthetic,
		IDTokenReplaced:  replaced,
		Tokens: Tokens{
			IDToken:      idToken,
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			SessionToken: sessionToken,
		},
		Warnings: warnings,
	}, nil
}

// buildSyntheticIDToken produces a three-segment JWT (alg "none") whose claims
// Codex can parse, used when the input has no real id_token. The header keeps
// a cpa_synthetic marker and the signature is the literal "synthetic", so a
// synthetic token is always identifiable as such.
func buildSyntheticIDToken(email, accountID, planType, userID string, expUnix int64, now time.Time) string {
	authClaim := map[string]any{"chatgpt_account_id": accountID}
	if planType != "" {
		authClaim["chatgpt_plan_type"] = planType
	}
	if userID != "" {
		authClaim["chatgpt_user_id"] = userID
		authClaim["user_id"] = userID
	}
	payload := map[string]any{
		"iat":           now.Unix(),
		"exp":           expUnix,
		openAIAuthClaim: authClaim,
	}
	if email != "" {
		payload["email"] = email
	}
	header := map[string]any{"alg": "none", "typ": "JWT", "cpa_synthetic": true}

	encode := func(value any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return encode(header) + "." + encode(payload) + "." + syntheticIDTokenSignature
}

func sourceFormatOf(rec map[string]any) string {
	platform, okPlatform := rec["platform"].(string)
	accountType, okType := rec["type"].(string)
	_, hasCredentials := rec["credentials"]
	if okPlatform && platform != "" && okType && accountType != "" && hasCredentials {
		return SourceSub2API
	}
	return SourceWebSession
}

func obj(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}

func firstNonEmpty(values ...any) string {
	for _, value := range values {
		if s, ok := value.(string); ok {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func unixSeconds(value any) int64 {
	number, ok := value.(float64)
	if !ok || number <= 0 {
		return 0
	}
	return int64(number)
}

// normalizeTimestamp accepts unix seconds/milliseconds and the usual ISO-8601
// shapes found in session files.
func normalizeTimestamp(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case float64:
		if typed <= 0 {
			return time.Time{}, false
		}
		if typed > 1e11 {
			typed /= 1000
		}
		return time.Unix(int64(typed), 0).UTC(), true
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return time.Time{}, false
		}
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			"2006-01-02",
		} {
			if t, err := time.Parse(layout, trimmed); err == nil {
				return t.UTC(), true
			}
		}
	}
	return time.Time{}, false
}
