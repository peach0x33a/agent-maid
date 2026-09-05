package gptsession

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// JWTPayload decodes the payload segment of a JWT without verifying the
// signature. Conversion only reads claims, and the access token came straight
// from the input file, so verification is meaningless here. Returns nil when
// the token is not a parseable JWT.
func JWTPayload(token string) map[string]any {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	raw, err := decodeBase64URL(parts[1])
	if err != nil {
		return nil
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return nil
	}
	return payload
}

func decodeBase64URL(segment string) ([]byte, error) {
	if raw, err := base64.RawURLEncoding.DecodeString(segment); err == nil {
		return raw, nil
	}
	return base64.URLEncoding.DecodeString(segment)
}

const (
	openAIAuthClaim    = "https://api.openai.com/auth"
	openAIProfileClaim = "https://api.openai.com/profile"
)

func openAIAuthClaimOf(payload map[string]any) map[string]any {
	return claimSection(payload, openAIAuthClaim)
}

func openAIProfileClaimOf(payload map[string]any) map[string]any {
	return claimSection(payload, openAIProfileClaim)
}

func claimSection(payload map[string]any, name string) map[string]any {
	if payload == nil {
		return nil
	}
	section, _ := payload[name].(map[string]any)
	return section
}

// AuthClaimOf exposes the OpenAI auth-claim section of a decoded payload for
// callers outside this package (plan type / user id resolution).
func AuthClaimOf(payload map[string]any) map[string]any {
	return openAIAuthClaimOf(payload)
}

// ProfileClaimOf exposes the OpenAI profile-claim section of a payload.
func ProfileClaimOf(payload map[string]any) map[string]any {
	return openAIProfileClaimOf(payload)
}
