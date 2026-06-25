// Command mgate-agent-sim 是一个【仅用于开发测试】的 agent 模拟器，支持两种通道：
//
//	--mode ws    （默认）WebSocket 长连接：hello / heartbeat / status，并响应 command.deliver。
//	--mode pull               HTTPS Pull 兜底：周期 POST /api/agent/pull，上报状态、拉取命令、提交 ack/result。
//
// 安全声明：本模拟器【不】实现任何设备控制能力——不调用 mgate.sh、不执行 shell、
// 不引入 os/exec。它只返回写死的模拟结果，是纯粹的协议联调工具。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

var (
	start   = time.Now()
	writeMu sync.Mutex // 串行化 WS 写：coder/websocket 不允许并发写
)

func main() {
	gateway := flag.String("gateway", "http://127.0.0.1:8080", "cloud 网关地址（http(s) 或 ws(s)）")
	deviceID := flag.String("device-id", "", "设备 ID（enroll 返回的 device_id）")
	token := flag.String("token", "", "设备令牌（enroll 返回的 device_token）")
	hostname := flag.String("hostname", "mgate-sim-001", "上报的 hostname")
	mode := flag.String("mode", "ws", "通道模式：ws 或 pull")
	heartbeat := flag.Duration("heartbeat", 25*time.Second, "WS 心跳间隔")
	pullInterval := flag.Duration("pull-interval", 5*time.Second, "Pull 轮询间隔")
	sendStatus := flag.Bool("status", true, "是否上报 status")
	flag.Parse()

	if *deviceID == "" || *token == "" {
		log.Fatal("必须提供 -device-id 与 -token")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch *mode {
	case "pull":
		runPull(ctx, *gateway, *deviceID, *token, *hostname, *pullInterval, *sendStatus)
	default:
		runWS(ctx, *gateway, *deviceID, *token, *hostname, *heartbeat, *sendStatus)
	}
}

// ── WS 模式 ──────────────────────────────────────────────────────────────

func runWS(ctx context.Context, gateway, deviceID, token, hostname string, heartbeat time.Duration, sendStatus bool) {
	url := wsURL(gateway)
	log.Printf("[ws] 连接 %s (device_id=%s)", url, deviceID)

	c, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: map[string][]string{
			"X-Mgate-Device-ID": {deviceID},
			"Authorization":     {"Bearer " + token},
		},
	})
	if err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "bye")
	log.Printf("[ws] 已连接")

	go wsReadLoop(ctx, c, deviceID)

	if err := wsSend(ctx, c, "agent.hello", deviceID, helloPayload(hostname)); err != nil {
		log.Fatalf("发送 hello 失败: %v", err)
	}
	if sendStatus {
		_ = wsSend(ctx, c, "agent.status", deviceID, sampleStatus())
	}

	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("[ws] 退出")
			return
		case <-ticker.C:
			if err := wsSend(ctx, c, "agent.heartbeat", deviceID, map[string]any{"uptime_sec": uptime()}); err != nil {
				log.Printf("[ws] 发送 heartbeat 失败: %v", err)
				return
			}
			log.Printf("[ws] → heartbeat")
		}
	}
}

func wsReadLoop(ctx context.Context, c *websocket.Conn, deviceID string) {
	for {
		var env map[string]json.RawMessage
		if err := wsjson.Read(ctx, c, &env); err != nil {
			return
		}
		var typ string
		_ = json.Unmarshal(env["type"], &typ)
		log.Printf("[ws] ← %s %s", typ, string(env["payload"]))
		if typ == "command.deliver" {
			go wsHandleCommand(ctx, c, deviceID, env["payload"])
		}
	}
}

func wsHandleCommand(ctx context.Context, c *websocket.Conn, deviceID string, payload json.RawMessage) {
	cmd := parseDeliver(payload)
	if cmd.CommandID == "" {
		return
	}
	_ = wsSend(ctx, c, "command.ack", deviceID, map[string]any{"command_id": cmd.CommandID, "accepted": true, "message": "accepted"})
	log.Printf("[ws] → command.ack %s", cmd.CommandID)
	select {
	case <-ctx.Done():
		return
	case <-time.After(200 * time.Millisecond):
	}
	stdout, result := simulateResult(cmd.Action)
	_ = wsSend(ctx, c, "command.result", deviceID, resultBody(cmd.CommandID, stdout, result))
	log.Printf("[ws] → command.result %s (action=%s)", cmd.CommandID, cmd.Action)
}

func wsSend(ctx context.Context, c *websocket.Conn, msgType, deviceID string, payload any) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	return wsjson.Write(ctx, c, envelope(msgType, deviceID, payload))
}

// ── Pull 模式 ────────────────────────────────────────────────────────────

// inboxItem 是已从 Pull 领取、待在下一轮提交 ack+result 的命令。
type inboxItem struct {
	CommandID string
	Action    string
}

func runPull(ctx context.Context, gateway, deviceID, token, hostname string, interval time.Duration, sendStatus bool) {
	url := pullURL(gateway)
	log.Printf("[pull] 轮询 %s (device_id=%s, interval=%s)", url, deviceID, interval)

	var inbox []inboxItem // 上一轮领取、本轮需提交 ack/result 的命令

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		// 构造请求：上报信息/状态 + 对上一轮命令提交 ack 与 result。
		req := map[string]any{
			"agent_version": "sim-0.1.0",
			"mgate_version": "0.3.7",
			"hostname":      hostname,
			"device_model":  "ufi",
			"firmware_info": "debian",
			"capabilities":  []string{"ap.status", "wlan.list", "doctor.full"},
			"max_commands":  5,
		}
		if sendStatus {
			req["status"] = sampleStatus()
		}
		if len(inbox) > 0 {
			acks := make([]map[string]any, 0, len(inbox))
			results := make([]map[string]any, 0, len(inbox))
			for _, it := range inbox {
				acks = append(acks, map[string]any{"command_id": it.CommandID, "accepted": true, "message": "accepted"})
				stdout, result := simulateResult(it.Action)
				results = append(results, resultBody(it.CommandID, stdout, result))
			}
			req["acks"] = acks
			req["results"] = results
			log.Printf("[pull] → 提交 %d 条 ack/result", len(inbox))
			inbox = nil
		}

		cmds, err := postPull(ctx, url, deviceID, token, req)
		if err != nil {
			log.Printf("[pull] 请求失败: %v", err)
		} else {
			for _, c := range cmds {
				log.Printf("[pull] ← command %s (action=%s)", c.CommandID, c.Action)
				inbox = append(inbox, inboxItem{CommandID: c.CommandID, Action: c.Action})
			}
		}

		select {
		case <-ctx.Done():
			log.Printf("[pull] 退出")
			return
		case <-ticker.C:
		}
	}
}

type pullCmd struct {
	CommandID string `json:"command_id"`
	Action    string `json:"action"`
}

func postPull(ctx context.Context, url, deviceID, token string, body map[string]any) ([]pullCmd, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mgate-Device-ID", deviceID)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{status: resp.StatusCode, body: string(data)}
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Commands []pullCmd `json:"commands"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return env.Data.Commands, nil
}

type httpError struct {
	status int
	body   string
}

func (e *httpError) Error() string { return "pull http " + itoa(e.status) + ": " + e.body }

// ── 共享辅助 ──────────────────────────────────────────────────────────────

func envelope(msgType, deviceID string, payload any) map[string]any {
	return map[string]any{
		"v":         1,
		"id":        "msg_" + msgType + "_" + time.Now().Format("150405.000"),
		"type":      msgType,
		"ts":        time.Now().UTC(),
		"device_id": deviceID,
		"payload":   payload,
	}
}

func helloPayload(hostname string) map[string]any {
	return map[string]any{
		"agent_version": "sim-0.1.0", "mgate_version": "0.3.7", "hostname": hostname,
		"device_model": "ufi", "firmware_info": "debian",
		"capabilities": []string{"ap.status", "wlan.list", "doctor.full"},
	}
}

func resultBody(commandID, stdout string, result map[string]any) map[string]any {
	now := time.Now().UTC()
	return map[string]any{
		"command_id": commandID, "status": "succeeded", "exit_code": 0,
		"stdout": stdout, "stderr": "", "result": result,
		"started_at": now.Add(-200 * time.Millisecond), "finished_at": now,
	}
}

func parseDeliver(payload json.RawMessage) pullCmd {
	var c pullCmd
	_ = json.Unmarshal(payload, &c)
	return c
}

// simulateResult 按 action 返回模拟的 stdout 与结构化结果（不执行任何真实命令）。
func simulateResult(action string) (stdout string, result map[string]any) {
	switch action {
	case "ap.status":
		return "ap running", map[string]any{"state": "running", "ssid": "Mgate-Sim"}
	case "gateway.status":
		return "gateway running", map[string]any{"state": "running"}
	case "doctor.full":
		return "simulated doctor ok", map[string]any{"checks": "ok"}
	case "wlan.list":
		return "wlan list", map[string]any{"networks": []string{"HomeWiFi", "Office"}}
	default:
		return "ok", map[string]any{"ok": true}
	}
}

func sampleStatus() map[string]any {
	return map[string]any{
		"ap":      map[string]any{"state": "running", "ssid": "Mgate-SIM"},
		"wlan":    map[string]any{"state": "connected", "ssid": "HomeWiFi"},
		"tproxy":  map[string]any{"state": "enabled", "current_node": "US"},
		"gateway": map[string]any{"state": "running"},
		"system":  map[string]any{"uptime_sec": uptime(), "load": []float64{0.2, 0.1, 0.1}},
	}
}

func uptime() int { return int(time.Since(start).Seconds()) }

func wsURL(gateway string) string   { return schemeSwap(gateway, true) + "/api/agent/ws" }
func pullURL(gateway string) string { return strings.TrimRight(gateway, "/") + "/api/agent/pull" }

// schemeSwap 把 http(s) 转为 ws(s)（toWS=true）。
func schemeSwap(gateway string, toWS bool) string {
	g := strings.TrimRight(gateway, "/")
	if !toWS {
		return g
	}
	switch {
	case strings.HasPrefix(g, "https://"):
		return "wss://" + strings.TrimPrefix(g, "https://")
	case strings.HasPrefix(g, "http://"):
		return "ws://" + strings.TrimPrefix(g, "http://")
	}
	return g
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
