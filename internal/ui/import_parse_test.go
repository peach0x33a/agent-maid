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

	// 语法损坏的整段 JSON:含可解析的完整行时保留,一个都拿不到时返回 0 文档
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

// 从终端粘贴多行 JSONL 时,textinput 会把换行折叠成空格;此时仍必须
// 把每一段 JSON 都解析出来(否则提示"未找到账号对象")。
func TestParseJSONDocsPastedSingleLine(t *testing.T) {
	pasted := []byte(`{"accessToken":"a","user":{"email":"a@b.c"}} {"accessToken":"b","user":{"email":"b@b.c"}}`)
	docs, bad, err := parseJSONDocs(pasted)
	if err != nil {
		t.Fatalf("粘贴单行多段: %v", err)
	}
	if len(docs) != 2 || bad != 0 {
		t.Fatalf("粘贴单行多段: docs=%d bad=%d, 期望 2/0", len(docs), bad)
	}
}

// 整包带缩进(官方导出的常见形态):不能因为逐行解析把整包拆坏。
func TestParseJSONDocsIndented(t *testing.T) {
	indented := []byte("{\n  \"accessToken\": \"a\",\n  \"user\": {\"email\": \"a@b.c\"}\n}\n")
	docs, bad, err := parseJSONDocs(indented)
	if err != nil || len(docs) != 1 || bad != 0 {
		t.Fatalf("缩进整包: docs=%d bad=%d err=%v", len(docs), bad, err)
	}
}
