package config

import "testing"

// TestProdRequiresAppSecret 验证生产模式下空 AppSecret 会被 Validate 拒绝，且不临时生成。
func TestProdRequiresAppSecret(t *testing.T) {
	clearEnv(t)
	t.Setenv(envMode, "prod")
	t.Setenv(envAppSecret, "")

	cfg := Load()
	if !cfg.IsProduction() {
		t.Fatal("MGATE_MODE=prod 应为生产模式")
	}
	if cfg.AppSecret != "" {
		t.Error("生产模式不应临时生成 AppSecret")
	}
	if err := cfg.Validate(); err == nil {
		t.Error("生产模式空 AppSecret 应校验失败")
	}
}

// TestProdWithAppSecretOK 验证生产模式提供 AppSecret 后校验通过。
func TestProdWithAppSecretOK(t *testing.T) {
	clearEnv(t)
	t.Setenv(envMode, "production")
	t.Setenv(envAppSecret, "fixed-secret-value")

	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Errorf("生产模式有 AppSecret 应通过校验: %v", err)
	}
}

// TestDevGeneratesAppSecret 验证 dev 模式空 AppSecret 会临时生成且校验通过。
func TestDevGeneratesAppSecret(t *testing.T) {
	clearEnv(t)
	t.Setenv(envMode, "dev")
	t.Setenv(envAppSecret, "")

	cfg := Load()
	if cfg.AppSecret == "" || !cfg.AppSecretGenerated {
		t.Error("dev 模式应临时生成 AppSecret 并标记 generated")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("dev 模式应通过校验: %v", err)
	}
}

// TestUnknownModeDefaultsToProd 验证未知模式按最安全策略当作生产。
func TestUnknownModeDefaultsToProd(t *testing.T) {
	clearEnv(t)
	t.Setenv(envMode, "weird-value")
	cfg := Load()
	if !cfg.IsProduction() {
		t.Error("未知模式应回退为 prod（最安全）")
	}
}

// TestDefaultModeIsDev 验证未显式设置时默认 dev（保证零配置本地可启动）。
func TestDefaultModeIsDev(t *testing.T) {
	clearEnv(t)
	t.Setenv(envMode, "")
	cfg := Load()
	if cfg.Mode != ModeDev {
		t.Errorf("默认模式应为 dev，实际 %q", cfg.Mode)
	}
}
