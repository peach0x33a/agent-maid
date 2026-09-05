// Package codexauth turns normalized accounts into the native Codex
// auth.json shape and writes them into the official data directory, so the
// official binary picks the credentials up unchanged — the same "projection"
// idea used by Cockpit Tools' Codex module.
package gptauth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const AuthModeChatGPT = "chatgpt"

const (
	keyAuthMode    = "auth_mode"
	keyAPIKey      = "OPENAI_API_KEY"
	keyTokens      = "tokens"
	keyLastRefresh = "last_refresh"
)

// Tokens mirrors Codex's own auth.json token object. Field order matters for
// byte-stable output that diffs cleanly against what Codex itself writes.
type Tokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}

type managedAuth struct {
	AuthMode     string  `json:"auth_mode"`
	OpenAIAPIKey *string `json:"OPENAI_API_KEY"`
	Tokens       Tokens  `json:"tokens"`
	LastRefresh  string  `json:"last_refresh"`
}

// BuildAuthFile renders the native auth.json for a ChatGPT (OAuth) login.
// A real refresh token is kept as-is; an empty one is written as "" exactly
// like the reference converter, so Codex does not mistake it for refreshable.
func BuildAuthFile(tokens Tokens, lastRefresh time.Time) ([]byte, error) {
	raw, err := marshalManaged(managedAuth{
		AuthMode:     AuthModeChatGPT,
		OpenAIAPIKey: nil,
		Tokens:       tokens,
		LastRefresh:  formatTimestamp(lastRefresh),
	})
	if err != nil {
		return nil, err
	}
	return prettyJSON(raw)
}

// MergeAuthFile writes the managed keys into an existing auth.json while
// preserving unknown keys (and their order) that other tooling may have put
// there. An existing OPENAI_API_KEY is cleared: switching to a ChatGPT
// account must not leave an API-key credential half-active behind it.
func MergeAuthFile(existing []byte, tokens Tokens, lastRefresh time.Time) ([]byte, error) {
	managedRaw, err := marshalManaged(managedAuth{
		AuthMode:     AuthModeChatGPT,
		OpenAIAPIKey: nil,
		Tokens:       tokens,
		LastRefresh:  formatTimestamp(lastRefresh),
	})
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(existing)) == 0 {
		return prettyJSON(managedRaw)
	}
	ordered, err := parseOrderedObject(existing)
	if err != nil {
		// Existing file is not a JSON object; start from the clean native
		// shape. The writer keeps a backup of the old bytes.
		return prettyJSON(managedRaw)
	}
	managed, err := parseOrderedObject(managedRaw)
	if err != nil {
		return nil, fmt.Errorf("内部错误：托管键序列化失败: %w", err)
	}
	for _, key := range managed.keys {
		ordered.set(key, managed.values[key])
	}
	return prettyJSON(ordered.compact())
}

func marshalManaged(managed managedAuth) ([]byte, error) {
	return json.Marshal(managed)
}

func formatTimestamp(t time.Time) string {
	return t.UTC().Truncate(time.Millisecond).Format(time.RFC3339Nano)
}

func prettyJSON(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// orderedObject preserves JSON object key order across a read-modify-write,
// so rewriting auth.json does not reshuffle keys Codex (or the user) placed.
type orderedObject struct {
	keys   []string
	values map[string]json.RawMessage
}

func parseOrderedObject(data []byte) (*orderedObject, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("不是 JSON 对象")
	}
	ordered := &orderedObject{values: make(map[string]json.RawMessage)}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errors.New("对象键不是字符串")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		ordered.set(key, raw)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return ordered, nil
}

func (o *orderedObject) set(key string, raw json.RawMessage) {
	if _, exists := o.values[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.values[key] = raw
}

func (o *orderedObject) compact() []byte {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyJSON, _ := json.Marshal(key)
		buf.Write(keyJSON)
		buf.WriteByte(':')
		buf.Write(o.values[key])
	}
	buf.WriteByte('}')
	return buf.Bytes()
}
