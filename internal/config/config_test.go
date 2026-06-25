package config

import (
	"testing"
	"time"
)

// TestLoadDefaults 验证在未设置任何环境变量时返回合理默认值。
func TestLoadDefaults(t *testing.T) {
	// t.Setenv 会在测试结束后自动还原，避免污染其他测试。
	clearEnv(t)

	cfg := Load()

	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr 默认值错误: got %q", cfg.HTTPAddr)
	}
	if cfg.DBPath != "./data/mgate-cloud.db" {
		t.Errorf("DBPath 默认值错误: got %q", cfg.DBPath)
	}
	if cfg.CookieSecure != false {
		t.Errorf("CookieSecure 默认值应为 false")
	}
	if cfg.SessionTTL != 168*time.Hour {
		t.Errorf("SessionTTL 默认值错误: got %s", cfg.SessionTTL)
	}
	if cfg.HasBootstrapAdmin() {
		t.Errorf("无管理员环境变量时 HasBootstrapAdmin 应为 false")
	}
}

// TestLoadFromEnv 验证环境变量能正确覆盖默认值。
func TestLoadFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv(envHTTPAddr, ":9090")
	t.Setenv(envCookieSecure, "true")
	t.Setenv(envSessionTTLHours, "24")
	t.Setenv(envAdminUsername, "root")
	t.Setenv(envAdminPassword, "s3cret")

	cfg := Load()

	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr 覆盖失败: got %q", cfg.HTTPAddr)
	}
	if !cfg.CookieSecure {
		t.Errorf("CookieSecure 应被覆盖为 true")
	}
	if cfg.SessionTTL != 24*time.Hour {
		t.Errorf("SessionTTL 覆盖失败: got %s", cfg.SessionTTL)
	}
	if !cfg.HasBootstrapAdmin() {
		t.Errorf("提供了用户名口令，HasBootstrapAdmin 应为 true")
	}
}

// TestStringRedactsPassword 确认 String() 不会泄露口令明文。
func TestStringRedactsPassword(t *testing.T) {
	clearEnv(t)
	t.Setenv(envAdminPassword, "super-secret-value")
	cfg := Load()

	s := cfg.String()
	if contains(s, "super-secret-value") {
		t.Fatalf("配置字符串泄露了口令明文: %s", s)
	}
	if !contains(s, "(set)") {
		t.Errorf("应以 (set) 表示口令已设置: %s", s)
	}
}

// clearEnv 清空所有相关环境变量，保证测试从干净状态开始。
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		envHTTPAddr, envDBPath, envBaseURL, envCookieSecure,
		envAdminUsername, envAdminPassword, envSessionTTLHours,
	} {
		t.Setenv(k, "")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
