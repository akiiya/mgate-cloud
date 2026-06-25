package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mgate-cloud/internal/app"
	"mgate-cloud/internal/config"
	"mgate-cloud/internal/db"
)

const (
	testAdminUser = "admin"
	testAdminPass = "test-password-123"
)

// testEnv 持有一次集成测试所需的服务器、客户端与底层数据库路径。
type testEnv struct {
	server *httptest.Server
	client *http.Client
	dbPath string
}

// newTestEnv 启动一个带 bootstrap 管理员的内存级测试服务。
//
// 使用临时文件数据库 + httptest 服务器 + 带 cookie jar 的客户端，
// 尽量贴近真实 HTTP 行为（会话/CSRF 都依赖 cookie）。
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "app_test.db")

	cfg := config.Config{
		HTTPAddr:      ":0",
		DBPath:        dbPath,
		BaseURL:       "http://127.0.0.1:8080",
		CookieSecure:  false,
		AdminUsername: testAdminUser,
		AdminPassword: testAdminPass,
		SessionTTL:    time.Hour,
		// Phase 2：设备身份相关设置（测试需显式提供，否则令牌字节数为 0）。
		PairingTTL:        30 * time.Minute,
		DeviceTokenBytes:  32,
		PairingTokenBytes: 32,
		AppSecret:         "test-app-secret",
		// Phase 3：WebSocket 相关设置（必须为正，否则 Hub reaper ticker 会 panic）。
		WSHeartbeatInterval: 25 * time.Second,
		WSOfflineAfter:      90 * time.Second,
		WSMaxMessageBytes:   65536,
		// Phase 4：命令队列设置（必须显式提供，否则 ResultMaxBytes=0 会把结果全部截断）。
		CommandDefaultTimeout: 60 * time.Second,
		CommandMaxTimeout:     300 * time.Second,
		CommandResultMaxBytes: 262144,
		CommandPendingTTL:     10 * time.Minute,
		CommandReaperInterval: 10 * time.Second,
		// Phase 5：Pull 与重试设置（PullMaxBodyBytes 必须为正，否则 Pull 请求体被拒）。
		PullDefaultInterval:       15 * time.Second,
		PullMaxCommands:           10,
		PullMaxBodyBytes:          131072,
		CommandDefaultMaxAttempts: 3,
		CommandLeaseSeconds:       60 * time.Second,
		CommandRetryBackoff:       10 * time.Second,
	}

	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("初始化应用失败: %v", err)
	}
	t.Cleanup(func() { application.Close() })

	srv := httptest.NewServer(application.Handler())
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	return &testEnv{
		server: srv,
		client: &http.Client{Jar: jar},
		dbPath: dbPath,
	}
}

// envelope 对应服务端统一响应信封。
type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// do 发起请求并解析信封，返回状态码与信封。
func (e *testEnv) do(t *testing.T, method, path, csrf, body string) (int, envelope) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.server.URL+path, reader)
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

// fetchCSRF 获取 CSRF 令牌（cookie 由 jar 自动保存）。
func (e *testEnv) fetchCSRF(t *testing.T) string {
	t.Helper()
	status, env := e.do(t, http.MethodGet, "/api/auth/csrf", "", "")
	if status != http.StatusOK {
		t.Fatalf("获取 CSRF 失败，状态码 %d", status)
	}
	var data struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("解析 CSRF 响应失败: %v", err)
	}
	if data.CSRFToken == "" {
		t.Fatal("CSRF 令牌为空")
	}
	return data.CSRFToken
}

// TestHealthz 验证健康检查可用。
func TestHealthz(t *testing.T) {
	env := newTestEnv(t)
	status, body := env.do(t, http.MethodGet, "/api/healthz", "", "")
	if status != http.StatusOK || !body.OK {
		t.Fatalf("healthz 异常: status=%d ok=%t", status, body.OK)
	}
}

// TestReadyz 验证就绪探测在 DB 可用时返回 200 ready。
func TestReadyz(t *testing.T) {
	env := newTestEnv(t)
	status, body := env.do(t, http.MethodGet, "/api/readyz", "", "")
	if status != http.StatusOK || !body.OK {
		t.Fatalf("readyz 异常: status=%d ok=%t", status, body.OK)
	}
}

// TestMeUnauthorized 验证未登录访问 /api/auth/me 返回 401。
func TestMeUnauthorized(t *testing.T) {
	env := newTestEnv(t)
	status, body := env.do(t, http.MethodGet, "/api/auth/me", "", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("期望 401，实际 %d", status)
	}
	if body.OK || body.Error == nil || body.Error.Code != "unauthorized" {
		t.Errorf("错误信封不符: %+v", body.Error)
	}
}

// TestLoginWithoutCSRF 验证缺少 CSRF 令牌时登录被拒（403）。
func TestLoginWithoutCSRF(t *testing.T) {
	env := newTestEnv(t)
	// 先取一次 CSRF 以确保 cookie 存在，但故意不在请求头携带令牌。
	env.fetchCSRF(t)

	status, body := env.do(t, http.MethodPost, "/api/auth/login", "",
		`{"username":"admin","password":"test-password-123"}`)
	if status != http.StatusForbidden {
		t.Fatalf("缺少 CSRF 头应返回 403，实际 %d", status)
	}
	if body.Error == nil || body.Error.Code != "csrf_failed" {
		t.Errorf("应为 csrf_failed: %+v", body.Error)
	}
}

// TestLoginFailure 验证错误口令返回统一凭据错误。
func TestLoginFailure(t *testing.T) {
	env := newTestEnv(t)
	csrf := env.fetchCSRF(t)

	status, body := env.do(t, http.MethodPost, "/api/auth/login", csrf,
		`{"username":"admin","password":"wrong-password"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("错误口令应返回 401，实际 %d", status)
	}
	if body.Error == nil || body.Error.Code != "invalid_credentials" {
		t.Errorf("应为 invalid_credentials: %+v", body.Error)
	}
}

// TestLoginSuccessAndMe 验证正确登录后可访问 /api/auth/me。
func TestLoginSuccessAndMe(t *testing.T) {
	env := newTestEnv(t)
	csrf := env.fetchCSRF(t)

	status, body := env.do(t, http.MethodPost, "/api/auth/login", csrf,
		`{"username":"admin","password":"test-password-123"}`)
	if status != http.StatusOK || !body.OK {
		t.Fatalf("登录应成功: status=%d ok=%t err=%+v", status, body.OK, body.Error)
	}

	// 登录后会话 cookie 已在 jar 中，me 应返回 200 与用户名。
	status, meBody := env.do(t, http.MethodGet, "/api/auth/me", "", "")
	if status != http.StatusOK {
		t.Fatalf("登录后 me 应返回 200，实际 %d", status)
	}
	var me struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(meBody.Data, &me); err != nil {
		t.Fatalf("解析 me 失败: %v", err)
	}
	if me.Username != testAdminUser {
		t.Errorf("用户名不符: got %q", me.Username)
	}
}

// TestLogout 验证登出后无法再访问受保护接口。
func TestLogout(t *testing.T) {
	env := newTestEnv(t)
	csrf := env.fetchCSRF(t)

	if status, _ := env.do(t, http.MethodPost, "/api/auth/login", csrf,
		`{"username":"admin","password":"test-password-123"}`); status != http.StatusOK {
		t.Fatalf("登录失败，状态 %d", status)
	}

	// 登出需要 CSRF 与会话。
	if status, _ := env.do(t, http.MethodPost, "/api/auth/logout", csrf, ""); status != http.StatusOK {
		t.Fatalf("登出应成功，状态 %d", status)
	}

	// 登出后 me 应返回 401。
	if status, _ := env.do(t, http.MethodGet, "/api/auth/me", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("登出后 me 应返回 401，实际 %d", status)
	}
}

// TestAuditLogsWritten 验证登录成功/失败、bootstrap 均写入了审计日志。
func TestAuditLogsWritten(t *testing.T) {
	env := newTestEnv(t)
	csrf := env.fetchCSRF(t)

	// 触发一次失败与一次成功。
	env.do(t, http.MethodPost, "/api/auth/login", csrf, `{"username":"admin","password":"wrong"}`)
	env.do(t, http.MethodPost, "/api/auth/login", csrf, `{"username":"admin","password":"test-password-123"}`)

	// 重新打开同一数据库文件做断言（WAL 下多连接读取安全）。
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer database.Close()

	for _, action := range []string{
		"system.bootstrap_admin.created",
		"admin.login.failed",
		"admin.login.success",
	} {
		var n int
		if err := database.QueryRow(
			"SELECT COUNT(*) FROM audit_logs WHERE action = ?;", action,
		).Scan(&n); err != nil {
			t.Fatalf("查询审计失败: %v", err)
		}
		if n == 0 {
			t.Errorf("应存在审计事件 %q", action)
		}
	}

	// 安全断言：审计中绝不出现口令明文。
	var leak int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM audit_logs WHERE metadata_json LIKE '%test-password-123%';",
	).Scan(&leak); err != nil {
		t.Fatalf("查询泄露检查失败: %v", err)
	}
	if leak != 0 {
		t.Errorf("审计 metadata 不应包含口令明文")
	}
}
