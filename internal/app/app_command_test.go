package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"

	"mgate-cloud/internal/db"
)

// readUntil 持续读取信封，直到遇到目标类型或超时。
func readUntil(t *testing.T, ctx context.Context, c *websocket.Conn, wantType string) json.RawMessage {
	t.Helper()
	for i := 0; i < 20; i++ {
		typ, payload := readEnvelope(t, ctx, c)
		if typ == wantType {
			return payload
		}
	}
	t.Fatalf("未在限定次数内收到 %q", wantType)
	return nil
}

// createCommandAPI 通过管理员 API 创建命令，返回 command_id 与初始状态。
func createCommandAPI(t *testing.T, env *testEnv, csrf, deviceID, body string) (string, string, int, string) {
	t.Helper()
	status, resp := env.do(t, http.MethodPost, "/api/admin/devices/"+deviceID+"/commands", csrf, body)
	if status != http.StatusOK {
		code := ""
		if resp.Error != nil {
			code = resp.Error.Code
		}
		return "", "", status, code
	}
	var data struct {
		Command struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"command"`
	}
	json.Unmarshal(resp.Data, &data)
	return data.Command.ID, data.Command.Status, status, ""
}

// commandStatusAPI 读取命令详情中的状态。
func commandStatusAPI(t *testing.T, env *testEnv, commandID string) string {
	t.Helper()
	_, body := env.do(t, http.MethodGet, "/api/admin/commands/"+commandID, "", "")
	var data struct {
		Command struct {
			Status string `json:"status"`
		} `json:"command"`
	}
	json.Unmarshal(body.Data, &data)
	return data.Command.Status
}

// waitCommandStatus 轮询命令状态直到等于期望值。
func waitCommandStatus(t *testing.T, env *testEnv, commandID, want string) {
	t.Helper()
	for i := 0; i < 60; i++ {
		if commandStatusAPI(t, env, commandID) == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("命令 %s 未在限定时间内变为 %q（当前 %q）", commandID, want, commandStatusAPI(t, env, commandID))
}

// connectAndHello 连接 WS 并完成 hello，返回连接与上下文（连接由调用方关闭）。
func connectAndHello(t *testing.T, env *testEnv, ctx context.Context, deviceID, token string) *websocket.Conn {
	t.Helper()
	c, _, err := dialWS(t, env, authHeaders(deviceID, token))
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	sendEnvelope(t, ctx, c, "agent.hello", deviceID, map[string]any{"agent_version": "1.0"})
	readUntil(t, ctx, c, "server.hello")
	return c
}

// TestCommandFullFlow 验证创建→投递→ack→result 的完整链路。
func TestCommandFullFlow(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := connectAndHello(t, env, ctx, deviceID, token)
	defer c.Close(websocket.StatusNormalClosure, "")

	// 等待设备在线。
	for i := 0; i < 40; i++ {
		if online, _ := deviceOnlineAndStatus(t, env, deviceID); online {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 创建命令 → 应被投递（sent）。
	cmdID, status, http1, _ := createCommandAPI(t, env, csrf, deviceID, `{"action":"ap.status","params":{}}`)
	if http1 != http.StatusOK {
		t.Fatalf("创建命令失败，状态 %d", http1)
	}
	if status != "sent" {
		t.Errorf("在线创建应为 sent，实际 %q", status)
	}

	// agent 端应收到 command.deliver。
	deliverPayload := readUntil(t, ctx, c, "command.deliver")
	var dp struct {
		CommandID string `json:"command_id"`
		Action    string `json:"action"`
	}
	json.Unmarshal(deliverPayload, &dp)
	if dp.CommandID != cmdID || dp.Action != "ap.status" {
		t.Fatalf("command.deliver 载荷不符: %s", string(deliverPayload))
	}

	// 安全断言：投递载荷不含任何可执行字段。
	for _, bad := range []string{"shell", "cmd", "script", "args", "argv", "raw"} {
		if containsKey(deliverPayload, bad) {
			t.Errorf("command.deliver 不应含危险字段 %q", bad)
		}
	}

	// ack accepted → acked
	sendEnvelope(t, ctx, c, "command.ack", deviceID, map[string]any{"command_id": cmdID, "accepted": true, "message": "ok"})
	waitCommandStatus(t, env, cmdID, "acked")

	// result succeeded → succeeded
	sendEnvelope(t, ctx, c, "command.result", deviceID, map[string]any{
		"command_id": cmdID, "status": "succeeded", "exit_code": 0,
		"stdout": "ok", "stderr": "", "result": map[string]any{"state": "running"},
	})
	waitCommandStatus(t, env, cmdID, "succeeded")

	// 详情应能回放结果。
	_, detail := env.do(t, http.MethodGet, "/api/admin/commands/"+cmdID, "", "")
	var d struct {
		Result *struct {
			Status string `json:"status"`
			Stdout string `json:"stdout"`
		} `json:"result"`
	}
	json.Unmarshal(detail.Data, &d)
	if d.Result == nil || d.Result.Status != "succeeded" || d.Result.Stdout != "ok" {
		t.Errorf("命令结果回放异常: %+v", d.Result)
	}

	// 审计完整（按 action 存在性断言：create/deliver 为 admin actor，ack/result 为 device actor）。
	assertAuditActionsExist(t, env, []string{"command.create", "command.deliver", "command.ack", "command.result"})
}

// assertAuditActionsExist 断言这些审计 action 至少各出现一次（不限 actor）。
func assertAuditActionsExist(t *testing.T, env *testEnv, actions []string) {
	t.Helper()
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer database.Close()
	for _, a := range actions {
		var n int
		if err := database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action=?`, a).Scan(&n); err != nil {
			t.Fatalf("查询审计失败: %v", err)
		}
		if n == 0 {
			t.Errorf("应存在审计事件 %q", a)
		}
	}
}

// TestCommandRejected 验证 ack rejected 使命令变 failed 并保存结果。
func TestCommandRejected(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := connectAndHello(t, env, ctx, deviceID, token)
	defer c.Close(websocket.StatusNormalClosure, "")

	cmdID, _, _, _ := createCommandAPI(t, env, csrf, deviceID, `{"action":"ap.status","params":{}}`)
	readUntil(t, ctx, c, "command.deliver")

	sendEnvelope(t, ctx, c, "command.ack", deviceID, map[string]any{"command_id": cmdID, "accepted": false, "message": "unsupported action"})
	waitCommandStatus(t, env, cmdID, "failed")
}

// TestCommandOfflineQueued 验证 Phase 5：离线 enabled 设备创建命令成功并进入 pending 队列。
func TestCommandOfflineQueued(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, _ := enrollDevice(t, env, csrf) // 已 enroll 但未连接 WS → 离线

	status, body := env.do(t, http.MethodPost, "/api/admin/devices/"+deviceID+"/commands", csrf, `{"action":"ap.status","params":{}}`)
	if status != http.StatusOK {
		t.Fatalf("离线设备创建命令应成功(200)，实际 %d", status)
	}
	var data struct {
		Command struct {
			Status string `json:"status"`
		} `json:"command"`
		DeliveryHint string `json:"delivery_hint"`
	}
	if err := json.Unmarshal(body.Data, &data); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if data.Command.Status != "pending" {
		t.Errorf("离线命令应为 pending，实际 %q", data.Command.Status)
	}
	if data.DeliveryHint != "device_offline_waiting_for_pull" {
		t.Errorf("离线 hint 应为 device_offline_waiting_for_pull，实际 %q", data.DeliveryHint)
	}
}

// TestCommandInvalidActionRejected 验证非白名单 action 被拒绝。
func TestCommandInvalidActionRejected(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := connectAndHello(t, env, ctx, deviceID, token)
	defer c.Close(websocket.StatusNormalClosure, "")
	for i := 0; i < 40; i++ {
		if online, _ := deviceOnlineAndStatus(t, env, deviceID); online {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	_, _, status, code := createCommandAPI(t, env, csrf, deviceID, `{"action":"exec.raw","params":{}}`)
	if status != http.StatusBadRequest || code != "invalid_action" {
		t.Errorf("非白名单应返回 invalid_action(400)，实际 status=%d code=%q", status, code)
	}
}

// TestCommandCancel 验证可取消未完成命令。
func TestCommandCancel(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := connectAndHello(t, env, ctx, deviceID, token)
	defer c.Close(websocket.StatusNormalClosure, "")
	for i := 0; i < 40; i++ {
		if online, _ := deviceOnlineAndStatus(t, env, deviceID); online {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cmdID, _, _, _ := createCommandAPI(t, env, csrf, deviceID, `{"action":"ap.status","params":{}}`)
	readUntil(t, ctx, c, "command.deliver")
	// sent 非终态，取消应成功。
	if status, _ := env.do(t, http.MethodPost, "/api/admin/commands/"+cmdID+"/cancel", csrf, ""); status != http.StatusOK {
		t.Fatalf("取消应成功，状态 %d", status)
	}
	if got := commandStatusAPI(t, env, cmdID); got != "canceled" {
		t.Errorf("应为 canceled，实际 %q", got)
	}
}

// TestCommandListAPI 验证命令列表返回创建的命令。
func TestCommandListAPI(t *testing.T) {
	env := newTestEnv(t)
	csrf := loginAsAdmin(t, env)
	deviceID, token := enrollDevice(t, env, csrf)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := connectAndHello(t, env, ctx, deviceID, token)
	defer c.Close(websocket.StatusNormalClosure, "")
	for i := 0; i < 40; i++ {
		if online, _ := deviceOnlineAndStatus(t, env, deviceID); online {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cmdID, _, _, _ := createCommandAPI(t, env, csrf, deviceID, `{"action":"ap.status","params":{}}`)

	_, body := env.do(t, http.MethodGet, "/api/admin/commands?device_id="+deviceID, "", "")
	var data struct {
		Items []struct {
			ID         string `json:"id"`
			DeviceName string `json:"device_name"`
			Action     string `json:"action"`
		} `json:"items"`
	}
	json.Unmarshal(body.Data, &data)
	found := false
	for _, it := range data.Items {
		if it.ID == cmdID {
			found = true
			if it.Action != "ap.status" {
				t.Errorf("action 不符: %q", it.Action)
			}
		}
	}
	if !found {
		t.Error("列表应包含创建的命令")
	}
}

// containsKey 判断 JSON 对象是否含某顶层键。
func containsKey(raw json.RawMessage, key string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}
