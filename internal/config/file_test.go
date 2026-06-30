package config

import (
	"path/filepath"
	"testing"
)

// TestSaveLoadRoundTrip 验证配置文件保存后可原样读回（含需转义的 app_secret）。
func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	tru, upd := true, true
	in := &FileConfig{
		HTTPAddr:           ":9090",
		BaseURL:            "https://cloud.example.com",
		DBPath:             "/var/lib/x.db",
		Mode:               "prod",
		CookieSecure:       &tru,
		AppSecret:          `se"cret\with/special+chars=`,
		AdminUsername:      "root",
		AdminPasswordHash:  "$2a$12$abcdefghijklmnopqrstuv",
		UpdateCheckEnabled: &upd,
		UpdateChannel:      "rc",
		GitHubRepo:         "akiiya/mgate-cloud",
	}
	if err := SaveFile(path, in); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	out, err := LoadFile(path)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if out.HTTPAddr != in.HTTPAddr || out.Mode != in.Mode || out.AppSecret != in.AppSecret {
		t.Errorf("字符串字段不一致: %+v", out)
	}
	if out.AdminPasswordHash != in.AdminPasswordHash {
		t.Errorf("password hash 不一致")
	}
	if out.CookieSecure == nil || *out.CookieSecure != true {
		t.Errorf("cookie_secure 应为 true")
	}
	if out.UpdateChannel != "rc" {
		t.Errorf("update_channel 不一致")
	}
}

// TestSaveDoesNotWritePlaintextPassword 验证保存不写 admin 明文密码。
func TestSaveDoesNotWritePlaintextPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	in := &FileConfig{AdminUsername: "admin", AdminPassword: "super-secret-plain", AdminPasswordHash: "$2a$hash"}
	if err := SaveFile(path, in); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	data, _ := LoadFile(path)
	if data.AdminPassword != "" {
		t.Error("配置文件不应包含明文密码")
	}
	if data.AdminPasswordHash == "" {
		t.Error("配置文件应包含 password hash")
	}
}

// TestResolveEnvOverridesFile 验证优先级：环境变量 > 文件 > 默认。
func TestResolveEnvOverridesFile(t *testing.T) {
	clearEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := SaveFile(path, &FileConfig{HTTPAddr: ":7000", Mode: "prod", AppSecret: "file-secret"}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	t.Setenv(envConfigPath, path)
	t.Setenv(envHTTPAddr, ":8888") // env 覆盖文件
	t.Setenv(envAppSecret, "")     // 未设置 → 用文件值
	t.Setenv(envMode, "")          // 未设置 → 用文件值

	cfg, info, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve 失败: %v", err)
	}
	if !info.FileExists {
		t.Error("应识别到配置文件存在")
	}
	if cfg.HTTPAddr != ":8888" {
		t.Errorf("env 应覆盖文件，HTTPAddr=%q", cfg.HTTPAddr)
	}
	if cfg.Mode != ModeProd {
		t.Errorf("Mode 应取自文件=prod，实际 %q", cfg.Mode)
	}
	if cfg.AppSecret != "file-secret" {
		t.Errorf("AppSecret 应取自文件，实际 %q", cfg.AppSecret)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("prod + 文件 app_secret 应通过校验: %v", err)
	}
}
