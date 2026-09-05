package gptauth

import "encoding/json"

// ReadCurrentTokens reads the token object and auth_mode out of the live
// auth.json, for "save current credentials into the library" flows. found is
// false when the file is missing or carries no usable tokens (e.g. API-key
// mode); a present but unparseable file is an error.
func ReadCurrentTokens(home string) (tokens Tokens, authMode string, found bool, err error) {
	data, err := ReadAuthFile(home)
	if err != nil {
		return Tokens{}, "", false, err
	}
	if data == nil {
		return Tokens{}, "", false, nil
	}
	ordered, err := parseOrderedObject(data)
	if err != nil {
		return Tokens{}, "", false, err
	}
	if raw, ok := ordered.values["auth_mode"]; ok {
		var mode string
		if json.Unmarshal(raw, &mode) == nil {
			authMode = mode
		}
	}
	rawTokens, ok := ordered.values["tokens"]
	if !ok {
		return Tokens{}, authMode, false, nil
	}
	if err := json.Unmarshal(rawTokens, &tokens); err != nil {
		return Tokens{}, authMode, false, nil
	}
	return tokens, authMode, tokens.AccessToken != "" || tokens.IDToken != "", nil
}
