// Package command 实现白名单命令队列：创建、投递、ack/result 回收与超时清理。
//
// 严格边界：cloud 只做"校验白名单 action → 落库 → 经 WS 投递 JSON → 保存结果"。
// 本包【不】执行任何命令、【不】拼接 shell、【不】引入 os/exec；真正调用设备本地
// mgate.sh 是 mgate-agent 的职责。
package command

import (
	"encoding/json"
	"errors"
	"time"

	"mgate-cloud/internal/model"
)

// ErrNotOnline 表示目标设备当前没有可用的 WebSocket 连接，无法投递。
var ErrNotOnline = errors.New("command: 设备不在线")

// DeliverPayload 是 cloud→agent 的 command.deliver 载荷。
//
// 刻意只含 command_id / action / params / timeout_sec —— 绝不包含 shell / cmd /
// script / args / argv / raw 等任何可执行字段。
type DeliverPayload struct {
	CommandID  string          `json:"command_id"`
	Action     string          `json:"action"`
	Params     json.RawMessage `json:"params"`
	TimeoutSec int             `json:"timeout_sec"`
}

// Deliverer 抽象"把一条命令投递到设备的在线连接"。
//
// 由 agent 包实现（持有 Hub 并负责构造 command.deliver 信封）。在此定义接口、在彼实现，
// 是为了避免 command→agent 的导入环（agent 需导入 command 以处理 ack/result）。
type Deliverer interface {
	Deliver(deviceID string, payload DeliverPayload) error
}

// AckInput 是 agent 上报的 command.ack 规整入参。
type AckInput struct {
	CommandID string
	Accepted  bool
	Message   string
}

// ResultInput 是 agent 上报的 command.result 规整入参。
type ResultInput struct {
	CommandID    string
	Status       string // succeeded / failed / timeout
	ExitCode     *int
	Stdout       string
	Stderr       string
	Result       json.RawMessage
	ErrorMessage string
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

// 终态集合：进入这些状态后不再变更（除审计外）。
func isTerminal(status string) bool {
	switch status {
	case model.CommandStatusSucceeded, model.CommandStatusFailed,
		model.CommandStatusTimeout, model.CommandStatusCanceled, model.CommandStatusExpired:
		return true
	default:
		return false
	}
}

// 合法的 agent 结果状态。
func isValidResultStatus(status string) bool {
	switch status {
	case model.CommandStatusSucceeded, model.CommandStatusFailed, model.CommandStatusTimeout:
		return true
	default:
		return false
	}
}
