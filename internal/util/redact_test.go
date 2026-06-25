package util

import "testing"

// TestRedactMasksSensitiveKeys 验证敏感字段被脱敏、普通字段保留。
func TestRedactMasksSensitiveKeys(t *testing.T) {
	in := map[string]any{
		"username":      "alice",
		"password":      "secret",
		"session_token": "abc",
		"CSRF":          "xyz",
		"note":          "hello",
	}
	out := Redact(in)

	if out["username"] != "alice" || out["note"] != "hello" {
		t.Error("普通字段不应被改动")
	}
	for _, k := range []string{"password", "session_token", "CSRF"} {
		if out[k] != "[REDACTED]" {
			t.Errorf("字段 %q 应被脱敏，实际 %v", k, out[k])
		}
	}
}

// TestRedactNilSafe 验证 nil 输入安全返回 nil。
func TestRedactNilSafe(t *testing.T) {
	if Redact(nil) != nil {
		t.Error("nil 输入应返回 nil")
	}
}
