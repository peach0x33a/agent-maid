package gptauth

import (
	"encoding/json"
	"time"

	"agent-maid/internal/gptsession"
)

// Current describes whatever credential is live in the official data
// directory right now, as read back from auth.json.
type Current struct {
	AuthMode        string
	AccountID       string
	Email           string
	LastRefresh     string
	LastRefreshTime *time.Time
	HasTokens       bool
}

// ReadCurrent parses the active auth.json. A missing file yields nil without
// error; a present but unparseable file yields an error.
func ReadCurrent(home string) (*Current, error) {
	data, err := ReadAuthFile(home)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	ordered, err := parseOrderedObject(data)
	if err != nil {
		return nil, err
	}
	current := &Current{}
	if raw, ok := ordered.values["auth_mode"]; ok {
		var mode string
		if err := json.Unmarshal(raw, &mode); err == nil {
			current.AuthMode = mode
		}
	}
	rawTokens, hasTokens := ordered.values["tokens"]
	if !hasTokens {
		return current, nil
	}
	var tokens struct {
		IDToken   string `json:"id_token"`
		AccountID string `json:"account_id"`
	}
	if err := json.Unmarshal(rawTokens, &tokens); err != nil {
		return current, nil
	}
	current.HasTokens = tokens.IDToken != "" || tokens.AccountID != ""
	current.AccountID = tokens.AccountID
	if payload := gptsession.JWTPayload(tokens.IDToken); payload != nil {
		if email, ok := payload["email"].(string); ok {
			current.Email = email
		}
	}
	if raw, ok := ordered.values["last_refresh"]; ok {
		var lastRefresh string
		if err := json.Unmarshal(raw, &lastRefresh); err == nil {
			current.LastRefresh = lastRefresh
			if t, ok := parseRFC3339(lastRefresh); ok {
				current.LastRefreshTime = &t
			}
		}
	}
	return current, nil
}

func parseRFC3339(value string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t, err = time.Parse(time.RFC3339, value)
	}
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
