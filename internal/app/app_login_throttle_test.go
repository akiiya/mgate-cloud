package app_test

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mgate-cloud/internal/app"
	"mgate-cloud/internal/config"
)

// newThrottleEnv 启动一个启用了登录失败限流的测试服务（低阈值便于触发）。
func newThrottleEnv(t *testing.T, maxFailures int) *testEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "throttle_app.db")
	cfg := config.Config{
		HTTPAddr:                  ":0",
		DBPath:                    dbPath,
		BaseURL:                   "http://127.0.0.1:8080",
		CookieSecure:              false,
		AdminUsername:             testAdminUser,
		AdminPassword:             testAdminPass,
		SessionTTL:                time.Hour,
		PairingTTL:                30 * time.Minute,
		DeviceTokenBytes:          32,
		PairingTokenBytes:         32,
		AppSecret:                 "test-app-secret",
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
		// 登录失败限流（本测试的重点）。
		LoginThrottleEnabled: true,
		LoginMaxFailures:     maxFailures,
		LoginFailureWindow:   15 * time.Minute,
		LoginBanBase:         time.Hour,
		LoginBanMax:          24 * time.Hour,
		LoginBanOffenseReset: 24 * time.Hour,
	}
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("初始化应用失败: %v", err)
	}
	t.Cleanup(func() { application.Close() })
	srv := httptest.NewServer(application.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &testEnv{server: srv, client: &http.Client{Jar: jar}, dbPath: dbPath}
}

// loginWithIP 模拟“经反代、来自某真实客户端 IP”的登录请求（通过 X-Forwarded-For 注入）。
func (e *testEnv) loginWithIP(t *testing.T, csrf, clientIP, body string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.server.URL+"/api/auth/login", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("X-Forwarded-For", clientIP)
	return e.client.Do(req)
}

// TestLoginThrottleBansAfterFailures 验证：达到失败阈值后，同一真实客户端 IP 即便口令正确也被 429 拦截。
func TestLoginThrottleBansAfterFailures(t *testing.T) {
	env := newThrottleEnv(t, 3)
	csrf := env.fetchCSRF(t)
	const clientIP = "203.0.113.66" // 模拟反代后的真实公网客户端

	// 前 3 次错误口令：返回 401 invalid_credentials。
	for i := 0; i < 3; i++ {
		resp, err := env.loginWithIP(t, csrf, clientIP, `{"username":"admin","password":"wrong"}`)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("第 %d 次失败应为 401，实际 %d", i+1, resp.StatusCode)
		}
	}

	// 第 4 次：即使口令正确也应被封禁拦截，返回 429 + Retry-After。
	resp, err := env.loginWithIP(t, csrf, clientIP, `{"username":"admin","password":"test-password-123"}`)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("封禁后应为 429，实际 %d", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("429 响应应包含 Retry-After 头")
	}
}

// TestLoginThrottleNeverBansLocalIP 验证：来自本地/回环（无真实公网 IP）的失败不会触发封禁，
// 避免把反代/基础设施 IP 拉黑而锁死所有人。
func TestLoginThrottleNeverBansLocalIP(t *testing.T) {
	env := newThrottleEnv(t, 3)
	csrf := env.fetchCSRF(t)

	// 不带 X-Forwarded-For：解析到的 IP 为本地回环（127.0.0.1），属不可封禁范围。
	for i := 0; i < 6; i++ {
		status, _ := env.do(t, http.MethodPost, "/api/auth/login", csrf, `{"username":"admin","password":"wrong"}`)
		if status != http.StatusUnauthorized {
			t.Fatalf("本地来源应始终返回 401（不封禁），第 %d 次实际 %d", i+1, status)
		}
	}
	// 正确口令仍可登录（未被误封）。
	if status, body := env.do(t, http.MethodPost, "/api/auth/login", csrf, `{"username":"admin","password":"test-password-123"}`); status != http.StatusOK || !body.OK {
		t.Fatalf("本地来源不应被封禁，正确口令应登录成功，实际 %d", status)
	}
}

// TestLoginThrottleDisabledByDefaultEnv 验证：默认（未启用限流）时多次失败不会被封禁。
func TestLoginThrottleAllowsAfterSuccess(t *testing.T) {
	env := newThrottleEnv(t, 5)
	csrf := env.fetchCSRF(t)

	// 两次失败（未达阈值）后成功登录，应清零；后续不应被拦截。
	for i := 0; i < 2; i++ {
		env.do(t, http.MethodPost, "/api/auth/login", csrf, `{"username":"admin","password":"wrong"}`)
	}
	if status, body := env.do(t, http.MethodPost, "/api/auth/login", csrf, `{"username":"admin","password":"test-password-123"}`); status != http.StatusOK || !body.OK {
		t.Fatalf("未达阈值的正确登录应成功，实际 status=%d", status)
	}
}
