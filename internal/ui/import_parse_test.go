package ui

import (
	"strings"
	"testing"
)

func TestParseJSONDocs(t *testing.T) {
	single := []byte(`{"accessToken":"a","user":{"email":"x@y.z"}}`)
	docs, bad, err := parseJSONDocs(single)
	if err != nil || len(docs) != 1 || bad != 0 {
		t.Fatalf("单个 JSON: docs=%d bad=%d err=%v", len(docs), bad, err)
	}

	jsonl := []byte(strings.Join([]string{
		`{"accessToken":"a","user":{"email":"a@b.c"}}`,
		`{"accessToken":"b","user":{"email":"b@b.c"}}`,
		`这不是 JSON`,
		``,
	}, "\n"))
	docs, bad, err = parseJSONDocs(jsonl)
	if err != nil {
		t.Fatalf("JSONL: %v", err)
	}
	if len(docs) != 2 || bad != 1 {
		t.Fatalf("JSONL: docs=%d bad=%d, 期望 2/1", len(docs), bad)
	}

	// 语法损坏的整段 JSON:退回逐行解析,保留可解析行
	broken := []byte("{\n  \"accessToken\": \"a\",\n  \"oops\"")
	docs, _, err = parseJSONDocs(broken)
	if err != nil || len(docs) != 0 {
		t.Fatalf("损坏 JSON: docs=%d err=%v", len(docs), err)
	}

	// 数组整包(sub2api 导出形状)
	arr := []byte(`[{"accessToken":"a","user":{"email":"a@b.c"}},{"accessToken":"b","user":{"email":"b@b.c"}}]`)
	docs, bad, err = parseJSONDocs(arr)
	if err != nil || len(docs) != 1 || bad != 0 {
		t.Fatalf("数组整包: docs=%d bad=%d err=%v", len(docs), bad, err)
	}
}
