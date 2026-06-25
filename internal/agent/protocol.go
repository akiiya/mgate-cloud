package agent

import (
	"encoding/json"
	"time"

	"mgate-cloud/internal/util"
)

// envelopeVersion 是当前协议版本；agent 必须以 v=1 发送。
const envelopeVersion = 1

// 消息类型。agent→cloud 仅支持 hello/heartbeat/status；cloud→agent 仅 server.hello/pong/error。
//
// 严禁新增任何"控制类"消息（command.deliver / exec.raw / shell / bash）：
// 本阶段只做连接与状态，不做控制。
const (
	typeAgentHello     = "agent.hello"
	typeAgentHeartbeat = "agent.heartbeat"
	typeAgentStatus    = "agent.status"

	typeServerHello = "server.hello"
	typeServerPong  = "server.pong"
	typeError       = "error"

	// Phase 4：命令通道。cloud→agent 投递；agent→cloud 确认与结果。
	typeCommandDeliver = "command.deliver"
	typeCommandAck     = "command.ack"
	typeCommandResult  = "command.result"
)

// Envelope 是统一 JSON 信封。payload 延迟解析（按 type 决定结构）。
type Envelope struct {
	V        int             `json:"v"`
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	TS       time.Time       `json:"ts"`
	DeviceID string          `json:"device_id"`
	Payload  json.RawMessage `json:"payload"`
}

// helloPayload 是 agent.hello 的载荷。
type helloPayload struct {
	AgentVersion string   `json:"agent_version"`
	MgateVersion string   `json:"mgate_version"`
	Hostname     string   `json:"hostname"`
	DeviceModel  string   `json:"device_model"`
	FirmwareInfo string   `json:"firmware_info"`
	Capabilities []string `json:"capabilities"`
}

// serverHelloPayload 是 server.hello 的载荷。
type serverHelloPayload struct {
	ServerTime           time.Time `json:"server_time"`
	HeartbeatIntervalSec int       `json:"heartbeat_interval_sec"`
}

// errorPayload 是 error 信封的载荷。
type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ackPayload 是 agent.command.ack 的载荷。
type ackPayload struct {
	CommandID string `json:"command_id"`
	Accepted  bool   `json:"accepted"`
	Message   string `json:"message"`
}

// resultPayload 是 agent.command.result 的载荷。
//
// result 字段为设备返回的结构化结果，延迟解析（原样保存）。
type resultPayload struct {
	CommandID    string          `json:"command_id"`
	Status       string          `json:"status"`
	ExitCode     *int            `json:"exit_code"`
	Stdout       string          `json:"stdout"`
	Stderr       string          `json:"stderr"`
	Result       json.RawMessage `json:"result"`
	ErrorMessage string          `json:"error_message"`
	StartedAt    *time.Time      `json:"started_at"`
	FinishedAt   *time.Time      `json:"finished_at"`
}

// newEnvelope 构造一条 cloud→agent 信封并序列化为 JSON 字节。
//
// id 自动生成（msg_ 前缀），ts 取当前时间，统一信封格式，避免各处手拼。
func newEnvelope(now time.Time, msgType, deviceID string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	env := Envelope{
		V:        envelopeVersion,
		ID:       "msg_" + util.NewID(),
		Type:     msgType,
		TS:       now,
		DeviceID: deviceID,
		Payload:  raw,
	}
	return json.Marshal(env)
}
