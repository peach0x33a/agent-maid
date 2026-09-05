package gptsession

import (
	"sort"
	"strconv"
)

// Record is one session/account object found inside an input document,
// together with where it was found (for user-facing messages).
type Record struct {
	Value      map[string]any
	SourceName string
	Path       string
}

const maxCollectDepth = 64

// Collect walks an arbitrary parsed JSON document and returns every object
// that looks like a ChatGPT session/account: it carries an access token
// (directly or nested under tokens/token/credentials) plus some identity hint.
// This mirrors the reference converter's collectSessionLikeObjects, so
// WebSession payloads, sub2api exports (accounts array or a bare account
// object) and similar shapes are all picked up without the caller having to
// know the format in advance.
func Collect(root any, sourceName string) []Record {
	var out []Record
	walk(root, "$", 0, sourceName, &out)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func walk(value any, path string, depth int, sourceName string, out *[]Record) {
	if depth > maxCollectDepth {
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		if accessTokenFrom(typed) != "" && hasIdentity(typed) {
			*out = append(*out, Record{Value: typed, SourceName: sourceName, Path: path})
			return
		}
		for key, child := range typed {
			// Skip token-valued keys so a token object (rare but possible)
			// is never treated as a session of its own.
			if key == "accessToken" || key == "access_token" || key == "sessionToken" {
				continue
			}
			walk(child, path+"."+key, depth+1, sourceName, out)
		}
	case []any:
		for i, child := range typed {
			walk(child, path+"["+strconv.Itoa(i)+"]", depth+1, sourceName, out)
		}
	}
}

func accessTokenFrom(rec map[string]any) string {
	tokens := obj(rec["tokens"])
	token := obj(rec["token"])
	creds := obj(rec["credentials"])
	return firstNonEmpty(
		rec["accessToken"], rec["access_token"],
		tokens["accessToken"], tokens["access_token"],
		token["accessToken"], token["access_token"],
		creds["accessToken"], creds["access_token"],
	)
}

func hasIdentity(rec map[string]any) bool {
	if _, ok := rec["user"].(map[string]any); ok {
		return true
	}
	tokens := obj(rec["tokens"])
	meta := obj(rec["meta"])
	psd := obj(rec["providerSpecificData"])
	return firstNonEmpty(
		rec["email"], rec["name"], rec["label"],
		meta["label"],
		tokens["accountId"], tokens["account_id"],
		tokens["chatgptAccountId"], tokens["chatgpt_account_id"],
		psd["chatgptAccountId"], psd["chatgpt_account_id"],
		rec["id"],
	) != ""
}
