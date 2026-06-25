package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"mgate-cloud/internal/db"
)

// enrollDevice 走完整链路创建并绑定一台设备，返回 device_id 与 device_token。
func enrollDevice(t *testing.T, env *testEnv, csrf string) (deviceID, token string) {
	t.Helper()
	deviceID = createDevice(t, env, csrf, "ws-device")
	code := generatePairingCode(t, env, csrf, deviceID)
	enrollBody := `{"device_code":"` + code + `","agent_version":"0.1.0","device_info":{"hostname":"h","model":"m","mgate_version":"v","firmware_info":"f"}}`
	status, body := env.do(t, http.MethodPost, "/api/agent/enroll", "", enrollBody)
	if status != http.StatusOK {
		t.Fatalf("enroll 失败，状态 %d", status)
	}
	var res struct {
		DeviceID    string `json:"device_id"`
		DeviceToken string `json:"device_token"`
	}
	if err := json.Unmarshal(body.Data, &res); err != nil {
		t.Fatalf("解析 enroll 响应失败: %v", err)
	}
	return res.DeviceID, res.DeviceToken
}

// wsURL 把 httptest 的 http(s) 地址转换为 ws 地址。
func wsURL(serverURL string) string {
	return "ws" + strings.TrimPrefix(serverURL, "http") + "/api/agent/ws"
}

// dialWS 以给定 header 连接 WebSocket。
func dialWS(t *testing.T, env *testEnv, headers http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return websocket.Dial(ctx, wsURL(env.server.URL), &websocket.DialOptions{HTTPHeader: headers})
}

// authHeaders 组装设备鉴权头。
func authHeaders(deviceID, token string) http.Header {
	h := http.Header{}
	if deviceID != "" {
		h.Set("X-Mgate-Device-ID", deviceID)
	}
	if token != "" {
		h.Set("Authorization", "Bearer "+token)
	}
	return h
}

// --- 鉴权用例 ---

func TestWSRejectsMissingDeviceID(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	_, token := enrollDevice(t, env, csrf)

	_, resp, err := dialWS(t, env, authHeaders("", token))
	if err == nil {
		t.Fatal("缺少 device_id 应连接失败")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("应返回 401，实际 %v", resp)
	}
}

func TestWSRejectsMissingAuthorization(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, _ := enrollDevice(t, env, csrf)

	_, resp, err := dialWS(t, env, authHeaders(deviceID, ""))
	if err == nil {
		t.Fatal("缺少 Authorization 应连接失败")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("应返回 401，实际 %v", resp)
	}
}

func TestWSRejectsWrongToken(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, _ := enrollDevice(t, env, csrf)

	_, resp, err := dialWS(t, env, authHeaders(deviceID, "mgdt_wrong-token"))
	if err == nil {
		t.Fatal("错误 token 应连接失败")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("应返回 401，实际 %v", resp)
	}
}

func TestWSRejectsPendingDevice(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID := createDevice(t, env, csrf, "pending-device") // 未 enroll，仍 pending

	_, resp, err := dialWS(t, env, authHeaders(deviceID, "mgdt_anything"))
	if err == nil {
		t.Fatal("pending 设备应连接失败")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("应返回 401，实际 %v", resp)
	}
}

func TestWSRejectsDisabledDevice(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	if status, _ := env.do(t, http.MethodPost, "/api/admin/devices/"+deviceID+"/disable", csrf, ""); status != http.StatusOK {
		t.Fatalf("禁用失败，状态 %d", status)
	}

	_, resp, err := dialWS(t, env, authHeaders(deviceID, token))
	if err == nil {
		t.Fatal("disabled 设备应连接失败")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("disabled 应返回 403，实际 %v", resp)
	}
}

func TestWSRejectsRevokedCredential(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	// 直接改库吊销凭证（Phase 3 无 revoke 接口，白盒方式构造场景）。
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	if _, err := database.Exec(`UPDATE device_credentials SET status='revoked', revoked_at=? WHERE device_id=?`, time.Now().UTC(), deviceID); err != nil {
		t.Fatalf("吊销凭证失败: %v", err)
	}
	database.Close()

	_, resp, err := dialWS(t, env, authHeaders(deviceID, token))
	if err == nil {
		t.Fatal("吊销凭证后应连接失败")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("应返回 401，实际 %v", resp)
	}
}

// --- 连接与消息用例 ---

// sendEnvelope 发送一条 agent→cloud 信封。
func sendEnvelope(t *testing.T, ctx context.Context, c *websocket.Conn, msgType, deviceID string, payload any) {
	t.Helper()
	env := map[string]any{
		"v":         1,
		"id":        "msg_" + msgType,
		"type":      msgType,
		"ts":        time.Now().UTC(),
		"device_id": deviceID,
		"payload":   payload,
	}
	if err := wsjson.Write(ctx, c, env); err != nil {
		t.Fatalf("发送 %s 失败: %v", msgType, err)
	}
}

// readEnvelope 读取一条 cloud→agent 信封，返回其 type。
func readEnvelope(t *testing.T, ctx context.Context, c *websocket.Conn) (string, json.RawMessage) {
	t.Helper()
	var env struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := wsjson.Read(ctx, c, &env); err != nil {
		t.Fatalf("读取信封失败: %v", err)
	}
	return env.Type, env.Payload
}

func TestWSConnectHelloHeartbeatStatusFlow(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, _, err := dialWS(t, env, authHeaders(deviceID, token))
	if err != nil {
		t.Fatalf("enabled 设备应连接成功: %v", err)
	}

	// hello → server.hello
	sendEnvelope(t, ctx, c, "agent.hello", deviceID, map[string]any{
		"agent_version": "1.2.3", "mgate_version": "0.9.0", "hostname": "ws-host",
		"device_model": "ufi", "firmware_info": "debian", "capabilities": []string{"ap.status", "wlan.list"},
	})
	if typ, _ := readEnvelope(t, ctx, c); typ != "server.hello" {
		t.Fatalf("应回 server.hello，实际 %q", typ)
	}

	// 在线状态应为 true。
	online, _ := deviceOnlineAndStatus(t, env, deviceID)
	if !online {
		t.Error("连接后设备应在线")
	}

	// hello 应更新设备字段。
	var dev struct {
		AgentVersion string `json:"agent_version"`
		Hostname     string `json:"hostname"`
	}
	_, detailRaw := deviceDetailRaw(t, env, deviceID)
	json.Unmarshal(detailRaw["device"], &dev)
	if dev.AgentVersion != "1.2.3" || dev.Hostname != "ws-host" {
		t.Errorf("hello 应更新设备信息，实际 %+v", dev)
	}

	// heartbeat → server.pong
	sendEnvelope(t, ctx, c, "agent.heartbeat", deviceID, map[string]any{"uptime_sec": 100})
	if typ, _ := readEnvelope(t, ctx, c); typ != "server.pong" {
		t.Fatalf("应回 server.pong，实际 %q", typ)
	}

	// status → 落库 latest_status
	sendEnvelope(t, ctx, c, "agent.status", deviceID, map[string]any{
		"ap": map[string]any{"state": "running", "ssid": "Mgate-XXXX"},
	})
	// status 无回包；轮询详情直到 latest_status 出现。
	var latestPresent bool
	for i := 0; i < 50; i++ {
		_, raw := deviceDetailRaw(t, env, deviceID)
		if ls, ok := raw["latest_status"]; ok && string(ls) != "null" {
			latestPresent = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !latestPresent {
		t.Error("status 上报后应保存 latest_status")
	}

	// 断开后应离线。
	c.Close(websocket.StatusNormalClosure, "")
	var offline bool
	for i := 0; i < 50; i++ {
		if online, _ := deviceOnlineAndStatus(t, env, deviceID); !online {
			offline = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !offline {
		t.Error("断开后设备应离线")
	}

	// 审计应包含连接/断开/hello/status 事件。
	assertAuditActions(t, env, deviceID, []string{
		"device.ws.connect", "device.ws.disconnect", "device.hello", "device.status.reported",
	})
}

func TestWSReplacesOldConnection(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c1, _, err := dialWS(t, env, authHeaders(deviceID, token))
	if err != nil {
		t.Fatalf("第一个连接失败: %v", err)
	}
	sendEnvelope(t, ctx, c1, "agent.hello", deviceID, map[string]any{"agent_version": "1"})
	readEnvelope(t, ctx, c1) // server.hello

	// 第二个连接接入，应取代第一个。
	c2, _, err := dialWS(t, env, authHeaders(deviceID, token))
	if err != nil {
		t.Fatalf("第二个连接失败: %v", err)
	}
	sendEnvelope(t, ctx, c2, "agent.hello", deviceID, map[string]any{"agent_version": "2"})
	readEnvelope(t, ctx, c2)

	// 第一个连接应被关闭：后续读取会失败。
	readCtx, rcancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer rcancel()
	var dummy map[string]any
	if err := wsjson.Read(readCtx, c1, &dummy); err == nil {
		t.Error("旧连接应已被新连接取代并关闭")
	}

	// 设备仍在线（由 c2 维持）。
	if online, _ := deviceOnlineAndStatus(t, env, deviceID); !online {
		t.Error("设备应仍在线（新连接维持）")
	}
	c2.Close(websocket.StatusNormalClosure, "")
}

func TestWSUnknownTypeDoesNotPanic(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, _, err := dialWS(t, env, authHeaders(deviceID, token))
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	// 未知类型应回 error 信封而非崩溃。
	sendEnvelope(t, ctx, c, "command.deliver", deviceID, map[string]any{"x": 1})
	if typ, _ := readEnvelope(t, ctx, c); typ != "error" {
		t.Errorf("未知类型应回 error，实际 %q", typ)
	}
	// 连接仍可用：发心跳应得到 pong。
	sendEnvelope(t, ctx, c, "agent.heartbeat", deviceID, map[string]any{})
	if typ, _ := readEnvelope(t, ctx, c); typ != "server.pong" {
		t.Errorf("连接应仍可用，期望 server.pong，实际 %q", typ)
	}
	c.Close(websocket.StatusNormalClosure, "")
}

func TestWSDeviceIDMismatchRejected(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, _, err := dialWS(t, env, authHeaders(deviceID, token))
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	// 信封 device_id 与连接认证设备不一致。
	sendEnvelope(t, ctx, c, "agent.heartbeat", "dev_other", map[string]any{})
	if typ, _ := readEnvelope(t, ctx, c); typ != "error" {
		t.Errorf("device_id 不匹配应回 error，实际 %q", typ)
	}
}

// --- 详情/在线辅助 ---

func deviceDetailRaw(t *testing.T, env *testEnv, deviceID string) (int, map[string]json.RawMessage) {
	t.Helper()
	status, body := env.do(t, http.MethodGet, "/api/admin/devices/"+deviceID, "", "")
	var m map[string]json.RawMessage
	json.Unmarshal(body.Data, &m)
	return status, m
}

func deviceOnlineAndStatus(t *testing.T, env *testEnv, deviceID string) (bool, json.RawMessage) {
	t.Helper()
	_, m := deviceDetailRaw(t, env, deviceID)
	var online bool
	json.Unmarshal(m["online"], &online)
	return online, m["latest_status"]
}

func assertAuditActions(t *testing.T, env *testEnv, deviceID string, actions []string) {
	t.Helper()
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer database.Close()
	for _, a := range actions {
		var n int
		if err := database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action=? AND actor_id=?`, a, deviceID).Scan(&n); err != nil {
			t.Fatalf("查询审计失败: %v", err)
		}
		if n == 0 {
			t.Errorf("应存在审计事件 %q", a)
		}
	}
	// 安全断言：审计中不得出现 device_token 前缀。
	var leak int
	database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE metadata_json LIKE '%mgdt_%'`).Scan(&leak)
	if leak != 0 {
		t.Error("审计 metadata 不应包含 device_token")
	}
}
