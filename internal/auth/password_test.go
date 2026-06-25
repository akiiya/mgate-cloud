package auth

import "testing"

// TestHashAndVerifyPassword 验证哈希可被正确校验，且错误口令被拒绝。
func TestHashAndVerifyPassword(t *testing.T) {
	const plain = "correct-horse-battery-staple"

	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("哈希失败: %v", err)
	}
	if hash == plain {
		t.Fatal("哈希结果不应等于明文")
	}

	if !VerifyPassword(hash, plain) {
		t.Error("正确口令应校验通过")
	}
	if VerifyPassword(hash, "wrong-password") {
		t.Error("错误口令应校验失败")
	}
}

// TestHashPasswordRejectsEmpty 验证空口令被拒绝。
func TestHashPasswordRejectsEmpty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Error("空口令应返回错误")
	}
}

// TestHashIsSalted 验证同一口令两次哈希结果不同（bcrypt 自带随机盐）。
func TestHashIsSalted(t *testing.T) {
	h1, _ := HashPassword("same-password")
	h2, _ := HashPassword("same-password")
	if h1 == h2 {
		t.Error("同一口令的两次哈希应因随机盐而不同")
	}
}
