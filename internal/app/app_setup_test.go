package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mgate-cloud/internal/app"
	"mgate-cloud/internal/config"
)

// setupEnv 是 setup 模式下的测试环境（无 bootstrap 管理员、无配置文件）。
type setupEnv struct {
	server     *httptest.Server
	client     *http.Client
	configPath string
}

func newSetupEnv(t *testing.T) *setupEnv {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml") // 不存在 → setup 模式

	cfg := config.Config{
		Mode:             "dev",
		HTTPAddr:         ":0",
		DBPath:           filepath.Join(dir, "setup.db"),
		BaseURL:          "http://127.0.0.1:8080",
		AppSecret:        "test-app-secret",
		ConfigPath:       configPath,
		ConfigFileExists: false,
		SessionTTL:       time.Hour,
		// 必要的运行参数（否则相关组件零值会异常）。
		PairingTTL:                30 * time.Minute,
		DeviceTokenBytes:          32,
		PairingTokenBytes:         32,
		WSHeartbeatInterval:       25 * time.Second,
		WSOfflineAfter:            90 * time.Second,
		WSMaxMessageBytes:         65536,
		CommandDefaultTimeout:     60 * time.Second,
		CommandMaxTimeout:         300 * time.Second,
		CommandResultMaxBytes:     262144,
		CommandPendingTTL:         10 * time.Minute,
		CommandReaperInterval:     10 * time.Second,
		PullDefaultInterval:       15 * time.Second,
		PullMaxCommands:           10,
		PullMaxBodyBytes:          131072,
		CommandDefaultMaxAttempts: 3,
		CommandLeaseSeconds:       60 * time.Second,
		CommandRetryBackoff:       10 * time.Second,
		UpdateCheckEnabled:        true,
		UpdateChannel:             "stable",
		GitHubRepo:                "akiiya/mgate-cloud",
	}

	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	t.Cleanup(func() { application.Close() })

	srv := httptest.NewServer(application.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &setupEnv{server: srv, client: &http.Client{Jar: jar}, configPath: configPath}
}

func (e *setupEnv) do(t *testing.T, method, path, csrf, body string) (int, envelope) {
	t.Helper()
	req, err := http.NewRequest(method, e.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	return resp.StatusCode, env
}

func TestSetupFlow(t *testing.T) {
	e := newSetupEnv(t)

	// 1) setup 状态：需要初始化。
	_, body := e.do(t, http.MethodGet, "/api/setup/status", "", "")
	var st struct {
		SetupRequired bool `json:"setup_required"`
	}
	json.Unmarshal(body.Data, &st)
	if !st.SetupRequired {
		t.Fatal("应处于 setup 模式")
	}

	// 2) setup 未完成时，受保护接口返回 setup_required。
	if status, b := e.do(t, http.MethodGet, "/api/admin/devices", "", ""); status != http.StatusConflict || b.Error == nil || b.Error.Code != "setup_required" {
		t.Errorf("受保护接口应返回 setup_required，status=%d err=%+v", status, b.Error)
	}
	// healthz 仍可访问。
	if status, _ := e.do(t, http.MethodGet, "/api/healthz", "", ""); status != http.StatusOK {
		t.Errorf("healthz 应可访问，status=%d", status)
	}

	// 3) 完成 setup。
	reqBody := `{"admin_username":"admin","admin_password":"setup-pass-123","admin_password_confirm":"setup-pass-123","mode":"dev"}`
	status, cb := e.do(t, http.MethodPost, "/api/setup/complete", "", reqBody)
	if status != http.StatusOK || !cb.OK {
		t.Fatalf("setup complete 应成功: status=%d err=%+v", status, cb.Error)
	}

	// 4) 配置文件已生成，且不含明文密码、含 hash。
	data, err := os.ReadFile(e.configPath)
	if err != nil {
		t.Fatalf("配置文件未生成: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "setup-pass-123") {
		t.Error("配置文件不应包含明文密码")
	}
	if !strings.Contains(content, "admin_password_hash") {
		t.Error("配置文件应包含 admin_password_hash")
	}
	if !strings.Contains(content, "app_secret") {
		t.Error("配置文件应包含 app_secret")
	}

	// 5) setup 已完成，不可重复。
	if status, _ := e.do(t, http.MethodPost, "/api/setup/complete", "", reqBody); status != http.StatusConflict {
		t.Errorf("重复 setup 应被拒，status=%d", status)
	}

	// 6) 状态变为不需要 setup。
	_, body2 := e.do(t, http.MethodGet, "/api/setup/status", "", "")
	json.Unmarshal(body2.Data, &st)
	if st.SetupRequired {
		t.Error("setup 完成后不应再需要 setup")
	}

	// 7) 用创建的管理员登录（无需重启）。
	csrf := ""
	_, cs := e.do(t, http.MethodGet, "/api/auth/csrf", "", "")
	var csd struct {
		CSRFToken string `json:"csrfToken"`
	}
	json.Unmarshal(cs.Data, &csd)
	csrf = csd.CSRFToken
	if status, _ := e.do(t, http.MethodPost, "/api/auth/login", csrf, `{"username":"admin","password":"setup-pass-123"}`); status != http.StatusOK {
		t.Fatalf("setup 后应能登录，status=%d", status)
	}
	// 登录后可访问受保护接口。
	if status, _ := e.do(t, http.MethodGet, "/api/admin/devices", "", ""); status != http.StatusOK {
		t.Errorf("登录后设备列表应 200，status=%d", status)
	}
}
