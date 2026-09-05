package gptsession

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func jwt(payload string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return header + "." + body + ".sig"
}

func websessionFixture() map[string]any {
	return map[string]any{
		"user":         map[string]any{"id": "user-test", "email": "mark@example.com"},
		"expires":      "2026-08-06T14:29:36.155Z",
		"account":      map[string]any{"id": "00000000-0000-4000-9000-000000000000", "planType": "plus"},
		"accessToken":  jwt(`{"exp":1780473960,"https://api.openai.com/auth":{"chatgpt_account_id":"00000000-0000-4000-9000-000000000000","chatgpt_plan_type":"plus","chatgpt_user_id":"user-test"}}`),
		"sessionToken": "sess-token",
		"authProvider": "openai",
	}
}

func TestConvertWebSessionWithoutRefreshToken(t *testing.T) {
	now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	account, err := Convert(Record{Value: websessionFixture(), SourceName: "session.json", Path: "$"}, now)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if account.AccountID != "00000000-0000-4000-9000-000000000000" {
		t.Errorf("AccountID = %q", account.AccountID)
	}
	if account.Email != "mark@example.com" {
		t.Errorf("Email = %q", account.Email)
	}
	if account.PlanType != "plus" {
		t.Errorf("PlanType = %q", account.PlanType)
	}
	if account.UserID != "user-test" {
		t.Errorf("UserID = %q", account.UserID)
	}
	if account.SourceFormat != SourceWebSession {
		t.Errorf("SourceFormat = %q", account.SourceFormat)
	}
	if account.Refreshable {
		t.Error("expected non-refreshable")
	}
	if account.AccessExpiresAt == nil || account.AccessExpiresAt.Unix() != 1780473960 {
		t.Errorf("AccessExpiresAt = %v", account.AccessExpiresAt)
	}
	if account.Tokens.RefreshToken != "" {
		t.Errorf("RefreshToken = %q, want empty", account.Tokens.RefreshToken)
	}
	if account.Tokens.SessionToken != "sess-token" {
		t.Errorf("SessionToken = %q", account.Tokens.SessionToken)
	}
	if !account.IDTokenSynthetic {
		t.Error("expected synthetic id_token")
	}
	parts := strings.Split(account.Tokens.IDToken, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		t.Fatalf("synthetic id_token must have 3 non-empty segments: %q", account.Tokens.IDToken)
	}
	if parts[2] != "synthetic" {
		t.Errorf("signature = %q, want %q", parts[2], "synthetic")
	}
	var header map[string]any
	raw, _ := base64.RawURLEncoding.DecodeString(parts[0])
	json.Unmarshal(raw, &header)
	if header["alg"] != "none" || header["cpa_synthetic"] != true {
		t.Errorf("header = %v", header)
	}
	var payload map[string]any
	raw, _ = base64.RawURLEncoding.DecodeString(parts[1])
	json.Unmarshal(raw, &payload)
	if payload["email"] != "mark@example.com" {
		t.Errorf("payload email = %v", payload["email"])
	}
	auth, _ := payload["https://api.openai.com/auth"].(map[string]any)
	if auth == nil || auth["chatgpt_account_id"] != "00000000-0000-4000-9000-000000000000" {
		t.Errorf("payload auth claim = %v", payload["https://api.openai.com/auth"])
	}
	if payload["exp"].(float64) != 1780473960 {
		t.Errorf("payload exp = %v", payload["exp"])
	}
}

func TestConvertWithRealRefreshAndIDTokenKeepsThem(t *testing.T) {
	now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	rec := map[string]any{
		"user":         map[string]any{"email": "mark@example.com"},
		"accessToken":  jwt(`{"exp":1780473960,"https://api.openai.com/auth":{"chatgpt_account_id":"acc-1"}}`),
		"refreshToken": "real-refresh-token",
		"idToken":      jwt(`{"email":"mark@example.com","https://api.openai.com/auth":{"chatgpt_account_id":"acc-1"}}`),
		"tokens":       map[string]any{"account_id": "acc-1"},
	}
	account, err := Convert(Record{Value: rec, SourceName: "x", Path: "$"}, now)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !account.Refreshable {
		t.Error("expected refreshable")
	}
	if account.AccessExpiresAt != nil {
		t.Errorf("refreshable account must not pin an expiry, got %v", account.AccessExpiresAt)
	}
	if account.IDTokenSynthetic {
		t.Error("real id_token must be kept")
	}
	if account.Tokens.RefreshToken != "real-refresh-token" {
		t.Errorf("RefreshToken = %q", account.Tokens.RefreshToken)
	}
	if account.Tokens.IDToken != rec["idToken"] {
		t.Errorf("IDToken = %q", account.Tokens.IDToken)
	}
}

func TestConvertSub2apiExport(t *testing.T) {
	now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	doc := map[string]any{
		"exported_at": "2026-08-01T00:00:00Z",
		"proxies":     []any{},
		"accounts": []any{
			map[string]any{
				"name":     "a@example.com",
				"platform": "openai",
				"type":     "oauth",
				"credentials": map[string]any{
					"access_token":  jwt(`{"exp":1789999999,"https://api.openai.com/auth":{"chatgpt_account_id":"acc-a"}}`),
					"refresh_token": "rt-a",
					"id_token":      jwt(`{"https://api.openai.com/auth":{"chatgpt_account_id":"acc-a"}}`),
					"account_id":    "acc-a",
					"email":         "a@example.com",
					"plan_type":     "pro",
				},
			},
			map[string]any{
				"name":     "b@example.com",
				"platform": "openai",
				"type":     "oauth",
				"credentials": map[string]any{
					"access_token":       jwt(`{"exp":1780000000,"https://api.openai.com/auth":{"chatgpt_account_id":"acc-b"}}`),
					"chatgpt_account_id": "acc-b",
					"email":              "b@example.com",
					"plan_type":          "free",
				},
			},
		},
	}
	records := Collect(doc, "s2a.json")
	if len(records) != 2 {
		t.Fatalf("Collect found %d records, want 2", len(records))
	}
	first, err := Convert(records[0], now)
	if err != nil {
		t.Fatalf("Convert a: %v", err)
	}
	if first.SourceFormat != SourceSub2API {
		t.Errorf("SourceFormat = %q", first.SourceFormat)
	}
	if !first.Refreshable || first.IDTokenSynthetic {
		t.Errorf("account a: refreshable=%v synthetic=%v", first.Refreshable, first.IDTokenSynthetic)
	}
	second, err := Convert(records[1], now)
	if err != nil {
		t.Fatalf("Convert b: %v", err)
	}
	if second.AccountID != "acc-b" || second.Email != "b@example.com" {
		t.Errorf("account b resolved to %q / %q", second.AccountID, second.Email)
	}
	if second.Refreshable || second.AccessExpiresAt == nil || second.AccessExpiresAt.Unix() != 1780000000 {
		t.Errorf("account b refreshable=%v expires=%v", second.Refreshable, second.AccessExpiresAt)
	}
}

func TestConvertReplacesMismatchedIDToken(t *testing.T) {
	now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	rec := map[string]any{
		"accessToken":  jwt(`{"exp":1780473960,"https://api.openai.com/auth":{"chatgpt_account_id":"acc-new"}}`),
		"refreshToken": "rt",
		"idToken":      jwt(`{"https://api.openai.com/auth":{"chatgpt_account_id":"acc-old"}}`),
		"tokens":       map[string]any{"account_id": "acc-new"},
	}
	account, err := Convert(Record{Value: rec, SourceName: "x", Path: "$"}, now)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !account.IDTokenReplaced || !account.IDTokenSynthetic {
		t.Errorf("expected replaced synthetic id_token, got replaced=%v synthetic=%v", account.IDTokenReplaced, account.IDTokenSynthetic)
	}
	payload := JWTPayload(account.Tokens.IDToken)
	auth, _ := payload["https://api.openai.com/auth"].(map[string]any)
	if auth["chatgpt_account_id"] != "acc-new" {
		t.Errorf("synthetic id_token claims %v, want acc-new", auth["chatgpt_account_id"])
	}
}

func TestConvertTrustsAccessTokenClaimOverStaleRecordID(t *testing.T) {
	now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	rec := map[string]any{
		"account":     map[string]any{"id": "acc-stale", "planType": "plus"},
		"accessToken": jwt(`{"exp":1780473960,"https://api.openai.com/auth":{"chatgpt_account_id":"acc-live"}}`),
	}
	account, err := Convert(Record{Value: rec, SourceName: "x", Path: "$"}, now)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if account.AccountID != "acc-live" {
		t.Errorf("AccountID = %q, want acc-live (access token claim wins)", account.AccountID)
	}
	found := false
	for _, warning := range account.Warnings {
		if strings.Contains(warning, "acc-stale") && strings.Contains(warning, "acc-live") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a mismatch warning, got %v", account.Warnings)
	}
}

func TestCollectFindsNestedAndArraySessions(t *testing.T) {
	sessions := []any{
		map[string]any{
			"email":       "late@example.com",
			"accessToken": jwt(`{"exp":1780473960,"https://api.openai.com/auth":{"chatgpt_account_id":"acc-late"}}`),
		},
		map[string]any{
			"email":       "early@example.com",
			"accessToken": jwt(`{"exp":1780000000,"https://api.openai.com/auth":{"chatgpt_account_id":"acc-early"}}`),
		},
	}
	records := Collect(sessions, "arr.json")
	if len(records) != 2 {
		t.Fatalf("Collect(array) = %d records, want 2", len(records))
	}
	if records[0].Path != "$[0]" || records[1].Path != "$[1]" {
		t.Errorf("paths = %q, %q", records[0].Path, records[1].Path)
	}

	nested := map[string]any{"wrapper": map[string]any{"session": websessionFixture()}}
	records = Collect(nested, "nested.json")
	if len(records) != 1 {
		t.Fatalf("Collect(nested) = %d records, want 1", len(records))
	}
	if records[0].Path != "$.wrapper.session" {
		t.Errorf("path = %q", records[0].Path)
	}
}

func TestCollectCodexAuthJSONShape(t *testing.T) {
	doc := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token":  jwt(`{"exp":1780473960,"https://api.openai.com/auth":{"chatgpt_account_id":"acc-1"}}`),
			"refresh_token": "rt",
			"account_id":    "acc-1",
		},
	}
	records := Collect(doc, "auth.json")
	if len(records) != 1 {
		t.Fatalf("Collect = %d records, want 1", len(records))
	}
	account, err := Convert(records[0], time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if account.AccountID != "acc-1" || !account.Refreshable {
		t.Errorf("AccountID=%q refreshable=%v", account.AccountID, account.Refreshable)
	}
}

func TestJWTPayloadInvalidInputs(t *testing.T) {
	for _, token := range []string{"", "not-a-jwt", "a.b", "x." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".y"} {
		if JWTPayload(token) != nil {
			t.Errorf("JWTPayload(%q) should be nil", token)
		}
	}
	if JWTPayload(jwt(`{"exp":1}`)) == nil {
		t.Error("valid JWT should parse")
	}
}
