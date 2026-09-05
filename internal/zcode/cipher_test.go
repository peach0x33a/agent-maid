package zcode

import (
	"os"
	"strings"
	"testing"
)

// 金标准:Node(与 Electron 同源 crypto)用 secret="test-secret" 加密
// "hello-世界-12345" 的产物,验证 Go 侧解密兼容。
const goldenFromNode = "enc:v1:Vvrw4uxxlifTBRoP.270BbyFzLwPF8xcbuIs1lw.NFNnbjU1N4FgfPbowgNMpkc4"

func TestDecryptGoldenFromNode(t *testing.T) {
	key := DeriveKey("test-secret")
	got, err := Decrypt(key, goldenFromNode)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if got != "hello-世界-12345" {
		t.Fatalf("解密结果不符: %q", got)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := DeriveKey("another-secret")
	plain := `{"id":"123","token":"abc"}` // 含中文/引号/换行的内容
	for _, p := range []string{plain, "短", strings.Repeat("x", 5000)} {
		enc, err := Encrypt(key, p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(enc, EncPrefix) {
			t.Fatalf("密文缺前缀: %q", enc[:20])
		}
		got, err := Decrypt(key, enc)
		if err != nil {
			t.Fatalf("roundtrip 失败: %v", err)
		}
		if got != p {
			t.Fatalf("roundtrip 内容不符")
		}
	}
}

func TestDecryptPlaintextPassthrough(t *testing.T) {
	// 无前缀的明文应原样返回(与 ZCode 行为一致)
	got, err := Decrypt(nil, "plain-value")
	if err != nil || got != "plain-value" {
		t.Fatalf("明文透传失败: %q %v", got, err)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	wrong := DeriveKey("other")
	if _, err := Decrypt(wrong, goldenFromNode); err == nil {
		t.Fatal("错误密钥应当解密失败")
	}
}

func TestSecretFallbackShape(t *testing.T) {
	os.Unsetenv("ZCODE_CREDENTIAL_SECRET")
	s := CredentialSecret()
	if !strings.HasPrefix(s, fallbackPrefix+":") {
		t.Fatalf("fallback secret 形状异常: %q", s)
	}
	t.Setenv("ZCODE_CREDENTIAL_SECRET", "env-override")
	if CredentialSecret() != "env-override" {
		t.Fatal("环境变量覆盖未生效")
	}
}
