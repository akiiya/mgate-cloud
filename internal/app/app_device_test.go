package app_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"mgate-cloud/internal/db"
)

// loginAsAdmin 完成 CSRF 获取与登录，返回可复用于后续写请求的 CSRF 令牌。
func loginAsAdmin(t *testing.T, env *testEnv) string {
	t.Helper()
	csrf := env.fetchCSRF(t)
	if status, _ := env.do(t, http.MethodPost, "/api/auth/login", csrf,
		`{"username":"admin","password":"test-password-123"}`); status != http.StatusOK {
		t.Fatalf("登录失败，状态 %d", status)
	}
	return csrf
}

// createDevice 创建设备并返回其 id。
func createDevice(t *testing.T, env *testEnv, csrf, name string) string {
	t.Helper()
	status, body := env.do(t, http.MethodPost, "/api/admin/devices", csrf,
		`{"name":"`+name+`","remark":"r"}`)
	if status != http.StatusOK {
		t.Fatalf("创建设备失败，状态 %d", status)
	}
	var data struct {
		Device struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"device"`
	}
	if err := json.Unmarshal(body.Data, &data); err != nil {
		t.Fatalf("解析创建响应失败: %v", err)
	}
	if data.Device.Status != "pending" {
		t.Errorf("新建设备应为 pending，实际 %q", data.Device.Status)
	}
	return data.Device.ID
}

// generatePairingCode 为设备生成设备码并返回明文设备码。
func generatePairingCode(t *testing.T, env *testEnv, csrf, deviceID string) string {
	t.Helper()
	status, body := env.do(t, http.MethodPost, "/api/admin/devices/"+deviceID+"/pairing-code", csrf, "")
	if status != http.StatusOK {
		t.Fatalf("生成设备码失败，状态 %d", status)
	}
	var data struct {
		DeviceCode string `json:"device_code"`
	}
	if err := json.Unmarshal(body.Data, &data); err != nil {
		t.Fatalf("解析设备码响应失败: %v", err)
	}
	if data.DeviceCode == "" {
		t.Fatal("设备码为空")
	}
	return data.DeviceCode
}

// TestDeviceCreateRequiresAuth 验证未登录无法访问设备接口。
func TestDeviceCreateRequiresAuth(t *testing.T) {
	env := newTestEnv(t)
	if status, _ := env.do(t, http.MethodGet, "/api/admin/devices", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("未登录列表应 401，实际 %d", status)
	}
}

// TestDeviceWriteRequiresCSRF 验证已登录但缺少 CSRF 头的写请求被拒（403）。
//
// 说明：本测试用 Go http.Client（不会跨请求复用自定义头），故 csrf="" 即真正不带头。
func TestDeviceWriteRequiresCSRF(t *testing.T) {
	env := newTestEnv(t)
	// 登录以获得会话 cookie；登录本身需要 CSRF。
	csrf := env.fetchCSRF(t)
	if status, _ := env.do(t, http.MethodPost, "/api/auth/login", csrf,
		`{"username":"admin","password":"test-password-123"}`); status != http.StatusOK {
		t.Fatalf("登录失败，状态 %d", status)
	}
	// 已登录，但创建设备时不带 CSRF 头 → 应 403。
	status, body := env.do(t, http.MethodPost, "/api/admin/devices", "", `{"name":"x"}`)
	if status != http.StatusForbidden {
		t.Fatalf("缺少 CSRF 的写请求应 403，实际 %d", status)
	}
	if body.Error == nil || body.Error.Code != "csrf_failed" {
		t.Errorf("应为 csrf_failed: %+v", body.Error)
	}
}

// TestDeviceCreateAndList 验证创建设备后出现在列表中。
func TestDeviceCreateAndList(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)

	id := createDevice(t, env, csrf, "办公室设备")

	status, body := env.do(t, http.MethodGet, "/api/admin/devices", "", "")
	if status != http.StatusOK {
		t.Fatalf("列表失败，状态 %d", status)
	}
	var data struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body.Data, &data); err != nil {
		t.Fatalf("解析列表失败: %v", err)
	}
	found := false
	for _, it := range data.Items {
		if it.ID == id {
			found = true
		}
	}
	if !found {
		t.Error("列表应包含刚创建的设备")
	}
}

// TestDeviceCreateEmptyNameFails 验证空名称创建失败。
func TestDeviceCreateEmptyNameFails(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	status, body := env.do(t, http.MethodPost, "/api/admin/devices", csrf, `{"name":"  ","remark":""}`)
	if status != http.StatusBadRequest {
		t.Fatalf("空名称应 400，实际 %d", status)
	}
	if body.Error == nil || body.Error.Code != "bad_request" {
		t.Errorf("应为 bad_request: %+v", body.Error)
	}
}

// TestEnrollFlowViaHTTP 验证完整链路：创建→设备码→enroll→设备 enabled。
func TestEnrollFlowViaHTTP(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)

	id := createDevice(t, env, csrf, "待绑定设备")
	code := generatePairingCode(t, env, csrf, id)

	// enroll 不需要 session / CSRF。
	enrollBody := `{"device_code":"` + code + `","agent_version":"0.1.0","device_info":{"hostname":"mgate-001","model":"ufi","mgate_version":"0.3.7","firmware_info":"debian"}}`
	status, body := env.do(t, http.MethodPost, "/api/agent/enroll", "", enrollBody)
	if status != http.StatusOK || !body.OK {
		t.Fatalf("enroll 应成功: status=%d err=%+v", status, body.Error)
	}
	var res struct {
		DeviceID    string `json:"device_id"`
		DeviceToken string `json:"device_token"`
	}
	if err := json.Unmarshal(body.Data, &res); err != nil {
		t.Fatalf("解析 enroll 响应失败: %v", err)
	}
	if res.DeviceID != id || res.DeviceToken == "" {
		t.Fatalf("enroll 返回异常: %+v", res)
	}

	// 详情应显示 enabled。
	_, detailBody := env.do(t, http.MethodGet, "/api/admin/devices/"+id, "", "")
	var detail struct {
		Device struct {
			Status string `json:"status"`
		} `json:"device"`
	}
	json.Unmarshal(detailBody.Data, &detail)
	if detail.Device.Status != "enabled" {
		t.Errorf("enroll 后应为 enabled，实际 %q", detail.Device.Status)
	}

	// 重复使用同一设备码应失败。
	if status, _ := env.do(t, http.MethodPost, "/api/agent/enroll", "", enrollBody); status == http.StatusOK {
		t.Error("重复 enroll 应失败")
	}
}

// TestEnrollAuditNoPlaintext 验证审计写入且不含设备码/令牌明文。
func TestEnrollAuditNoPlaintext(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	id := createDevice(t, env, csrf, "审计设备")
	code := generatePairingCode(t, env, csrf, id)

	enrollBody := `{"device_code":"` + code + `","agent_version":"0.1.0","device_info":{"hostname":"h","model":"m","mgate_version":"v","firmware_info":"f"}}`
	if status, _ := env.do(t, http.MethodPost, "/api/agent/enroll", "", enrollBody); status != http.StatusOK {
		t.Fatalf("enroll 失败，状态 %d", status)
	}

	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer database.Close()

	// 关键审计事件均已写入。
	for _, action := range []string{"device.create", "pairing_code.create", "device.enroll.success"} {
		var n int
		if err := database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = ?`, action).Scan(&n); err != nil {
			t.Fatalf("查询审计失败: %v", err)
		}
		if n == 0 {
			t.Errorf("应存在审计事件 %q", action)
		}
	}

	// 安全断言：任何审计 metadata 都不得包含设备码明文。
	var leak int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE metadata_json LIKE ?`, "%"+code+"%",
	).Scan(&leak); err != nil {
		t.Fatalf("查询泄露失败: %v", err)
	}
	if leak != 0 {
		t.Error("审计 metadata 不应包含设备码明文")
	}

	// 安全断言：审计中不得包含 device_token 前缀明文。
	var tokenLeak int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE metadata_json LIKE '%mgdt_%'`,
	).Scan(&tokenLeak); err != nil {
		t.Fatalf("查询 token 泄露失败: %v", err)
	}
	if tokenLeak != 0 {
		t.Error("审计 metadata 不应包含 device_token")
	}
}
