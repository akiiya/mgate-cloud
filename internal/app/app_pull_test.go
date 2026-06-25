package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"mgate-cloud/internal/db"
)

// doPull 发起一次 Pull 请求（使用独立 client，避免 cookie 干扰），返回状态码与信封。
func doPull(t *testing.T, env *testEnv, deviceID, token, body string) (int, envelope) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, env.server.URL+"/api/agent/pull", strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造 Pull 请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if deviceID != "" {
		req.Header.Set("X-Mgate-Device-ID", deviceID)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("Pull 请求失败: %v", err)
	}
	defer resp.Body.Close()
	var e envelope
	_ = json.NewDecoder(resp.Body).Decode(&e)
	return resp.StatusCode, e
}

// pullCommands 解析 Pull 响应中的命令列表。
func pullCommands(t *testing.T, e envelope) []struct {
	CommandID string `json:"command_id"`
	Action    string `json:"action"`
} {
	t.Helper()
	var data struct {
		Commands []struct {
			CommandID string `json:"command_id"`
			Action    string `json:"action"`
		} `json:"commands"`
	}
	json.Unmarshal(e.Data, &data)
	return data.Commands
}

// --- 鉴权 ---

func TestPullRejectsMissingDeviceID(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	_, token := enrollDevice(t, env, csrf)
	if status, _ := doPull(t, env, "", token, `{}`); status != http.StatusUnauthorized {
		t.Errorf("缺 device_id 应 401，实际 %d", status)
	}
}

func TestPullRejectsMissingAuth(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, _ := enrollDevice(t, env, csrf)
	if status, _ := doPull(t, env, deviceID, "", `{}`); status != http.StatusUnauthorized {
		t.Errorf("缺 Authorization 应 401，实际 %d", status)
	}
}

func TestPullRejectsWrongToken(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, _ := enrollDevice(t, env, csrf)
	if status, _ := doPull(t, env, deviceID, "mgdt_wrong", `{}`); status != http.StatusUnauthorized {
		t.Errorf("错误 token 应 401，实际 %d", status)
	}
}

func TestPullRejectsPendingDevice(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID := createDevice(t, env, csrf, "pending-pull") // 未 enroll
	if status, _ := doPull(t, env, deviceID, "mgdt_any", `{}`); status != http.StatusUnauthorized {
		t.Errorf("pending 设备应 401，实际 %d", status)
	}
}

func TestPullRejectsDisabledDevice(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)
	if status, _ := env.do(t, http.MethodPost, "/api/admin/devices/"+deviceID+"/disable", csrf, ""); status != http.StatusOK {
		t.Fatalf("禁用失败，状态 %d", status)
	}
	if status, _ := doPull(t, env, deviceID, token, `{}`); status != http.StatusForbidden {
		t.Errorf("disabled 设备应 403，实际 %d", status)
	}
}

func TestPullRejectsRevokedCredential(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)
	database, _ := db.Open(env.dbPath)
	database.Exec(`UPDATE device_credentials SET status='revoked', revoked_at=? WHERE device_id=?`, time.Now().UTC(), deviceID)
	database.Close()
	if status, _ := doPull(t, env, deviceID, token, `{}`); status != http.StatusUnauthorized {
		t.Errorf("吊销凭证应 401，实际 %d", status)
	}
}

// --- 心跳 / 状态 ---

func TestPullUpdatesLastPullAndStatus(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	body := `{"agent_version":"0.1.0","status":{"ap":{"state":"running"}}}`
	if status, _ := doPull(t, env, deviceID, token, body); status != http.StatusOK {
		t.Fatalf("Pull 应成功，实际 %d", status)
	}

	_, m := deviceDetailRaw(t, env, deviceID)
	var dev struct {
		LastPullAt   *string `json:"last_pull_at"`
		LastSeenAt   *string `json:"last_seen_at"`
		AgentVersion string  `json:"agent_version"`
	}
	json.Unmarshal(m["device"], &dev)
	if dev.LastPullAt == nil {
		t.Error("应更新 last_pull_at")
	}
	if dev.LastSeenAt == nil {
		t.Error("应更新 last_seen_at")
	}
	// latest_status source 应为 pull。
	var src string
	json.Unmarshal(m["latest_status_source"], &src)
	if src != "pull" {
		t.Errorf("latest_status_source 应为 pull，实际 %q", src)
	}
}

func TestPullRejectsUnknownField(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)
	if status, body := doPull(t, env, deviceID, token, `{"bogus_field":1}`); status != http.StatusBadRequest {
		t.Errorf("未知字段应 400，实际 %d (%+v)", status, body.Error)
	}
}

func TestPullRejectsOversizedBody(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)
	big := `{"hostname":"` + strings.Repeat("A", 200000) + `"}` // 超过 128 KiB
	if status, _ := doPull(t, env, deviceID, token, big); status == http.StatusOK {
		t.Error("超大请求体应被拒绝")
	}
}

// --- 命令拉取 / ack / result ---

func TestPullDeliversOfflineCommand(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf) // 离线

	// 离线创建命令 → pending。
	cmdID, status, _, _ := createCommandAPI(t, env, csrf, deviceID, `{"action":"ap.status","params":{}}`)
	if status != "pending" {
		t.Fatalf("离线命令应为 pending，实际 %q", status)
	}

	// Pull 领取命令。
	_, e := doPull(t, env, deviceID, token, `{"max_commands":5}`)
	cmds := pullCommands(t, e)
	if len(cmds) != 1 || cmds[0].CommandID != cmdID {
		t.Fatalf("Pull 应领取到该命令，实际 %d", len(cmds))
	}
	if commandStatusAPI(t, env, cmdID) != "sent" {
		t.Errorf("领取后应为 sent，实际 %q", commandStatusAPI(t, env, cmdID))
	}

	// Pull 提交 ack + result。
	doPull(t, env, deviceID, token, `{"acks":[{"command_id":"`+cmdID+`","accepted":true}]}`)
	if commandStatusAPI(t, env, cmdID) != "acked" {
		t.Errorf("ack 后应为 acked，实际 %q", commandStatusAPI(t, env, cmdID))
	}
	doPull(t, env, deviceID, token, `{"results":[{"command_id":"`+cmdID+`","status":"succeeded","stdout":"ok","result":{"state":"running"}}]}`)
	if commandStatusAPI(t, env, cmdID) != "succeeded" {
		t.Errorf("result 后应为 succeeded，实际 %q", commandStatusAPI(t, env, cmdID))
	}

	// 审计应含 device.pull 与 command.result。
	assertAuditActionsExist(t, env, []string{"device.pull", "command.result"})
}

func TestPullDoesNotReturnOtherDeviceCommand(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	devA, tokenA := enrollDevice(t, env, csrf)
	devB := createDevice(t, env, csrf, "dev-b")
	codeB := generatePairingCode(t, env, csrf, devB)
	enrollB := `{"device_code":"` + codeB + `","agent_version":"0.1.0","device_info":{"hostname":"b","model":"m","mgate_version":"v","firmware_info":"f"}}`
	env.do(t, http.MethodPost, "/api/agent/enroll", "", enrollB)

	// 给 devB 创建命令。
	createCommandAPI(t, env, csrf, devB, `{"action":"ap.status","params":{}}`)

	// devA Pull 不应拿到 devB 的命令。
	_, e := doPull(t, env, devA, tokenA, `{"max_commands":5}`)
	if cmds := pullCommands(t, e); len(cmds) != 0 {
		t.Errorf("不应领取其他设备命令，实际 %d", len(cmds))
	}
	_ = devA
}

func TestPullAndWSNoDoubleDelivery(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	// 离线创建一条 pending 命令。
	cmdID, _, _, _ := createCommandAPI(t, env, csrf, deviceID, `{"action":"ap.status","params":{}}`)

	// WS 连接 + hello → 重连投递应领取该命令（WS 通道）。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := connectAndHello(t, env, ctx, deviceID, token)
	defer c.Close(websocket.StatusNormalClosure, "")

	// WS 应收到 command.deliver。
	deliver := readUntil(t, ctx, c, "command.deliver")
	var dp struct {
		CommandID string `json:"command_id"`
	}
	json.Unmarshal(deliver, &dp)
	if dp.CommandID != cmdID {
		t.Fatalf("WS 应投递该命令")
	}

	// 此时命令已 sent；Pull 不应再次领取同一命令。
	_, e := doPull(t, env, deviceID, token, `{"max_commands":5}`)
	if cmds := pullCommands(t, e); len(cmds) != 0 {
		t.Errorf("已由 WS 投递的命令不应被 Pull 重复领取，实际 %d", len(cmds))
	}
}
