package auth

import "testing"

// TestGenerateTokenLength 验证生成的令牌具备足够长度（高熵）。
//
// 32 字节经 base64 RawURL 编码后约 43 个字符；此处用保守下界断言。
func TestGenerateTokenLength(t *testing.T) {
	token, err := generateToken()
	if err != nil {
		t.Fatalf("生成令牌失败: %v", err)
	}
	if len(token) < 40 {
		t.Errorf("令牌长度过短: %d", len(token))
	}
}

// TestGenerateTokenUnique 验证多次生成不会重复（随机性基础检查）。
func TestGenerateTokenUnique(t *testing.T) {
	seen := make(map[string]bool)
	const n = 1000
	for i := 0; i < n; i++ {
		token, err := generateToken()
		if err != nil {
			t.Fatalf("生成令牌失败: %v", err)
		}
		if seen[token] {
			t.Fatalf("出现重复令牌，随机性不足")
		}
		seen[token] = true
	}
}

// TestHashTokenStableAndOneWay 验证哈希对同一输入稳定、且不等于原文。
func TestHashTokenStableAndOneWay(t *testing.T) {
	const token = "some-raw-token"
	h1 := hashToken(token)
	h2 := hashToken(token)
	if h1 != h2 {
		t.Error("同一令牌的哈希应稳定一致")
	}
	if h1 == token {
		t.Error("哈希不应等于原始令牌")
	}
}
