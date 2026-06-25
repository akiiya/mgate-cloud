package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/command"
	"mgate-cloud/internal/device"
	"mgate-cloud/internal/model"
)

// PullHandlers 处理 agent 的 HTTPS Pull 兜底通道。
//
// Pull 与 WebSocket 共用同一套设备鉴权与命令 lease 机制，是 agent 无法保持长连接时的备选。
type PullHandlers struct {
	devices         *device.Service
	commands        *command.Service
	audit           *audit.Service
	maxBody         int64
	maxCommands     int
	defaultInterval time.Duration
}

// NewPullHandlers 构造 Pull 处理器。
func NewPullHandlers(deviceService *device.Service, commandService *command.Service, auditService *audit.Service, maxBody int64, maxCommands int, defaultInterval time.Duration) *PullHandlers {
	return &PullHandlers{
		devices:         deviceService,
		commands:        commandService,
		audit:           auditService,
		maxBody:         maxBody,
		maxCommands:     maxCommands,
		defaultInterval: defaultInterval,
	}
}

// pullRequest 是 Pull 请求体。status 为 freeform JSON（延迟解析）。
type pullRequest struct {
	AgentVersion string          `json:"agent_version"`
	MgateVersion string          `json:"mgate_version"`
	Hostname     string          `json:"hostname"`
	DeviceModel  string          `json:"device_model"`
	FirmwareInfo string          `json:"firmware_info"`
	Capabilities []string        `json:"capabilities"`
	Status       json.RawMessage `json:"status"`
	Acks         []ackPayload    `json:"acks"`
	Results      []resultPayload `json:"results"`
	MaxCommands  int             `json:"max_commands"`
}

// Pull 处理 POST /api/agent/pull。
//
// 处理顺序：鉴权 → 解析 → 更新设备/last_pull → 状态 → acks → results → 领取命令 → 响应。
// 与 WS 相同的 device_id + device_token 鉴权；鉴权在处理任何业务前完成。
func (h *PullHandlers) Pull(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimSpace(r.Header.Get(headerDeviceID))
	token := bearerToken(r.Header.Get(headerAuth))
	ip := audit.ClientIP(r)
	ua := r.UserAgent()
	requestID := audit.RequestIDFrom(r.Context())

	if deviceID == "" || token == "" {
		h.auditAuthFailed(r.Context(), deviceID, "missing_credentials", ip, ua, requestID)
		api.WriteError(w, api.ErrUnauthorized)
		return
	}
	dev, err := h.devices.AuthenticateDevice(r.Context(), deviceID, token)
	if err != nil {
		h.auditAuthFailed(r.Context(), deviceID, errCode(err), ip, ua, requestID)
		api.WriteError(w, err)
		return
	}

	// 限制请求体大小并严格解析（拒绝未知字段）。
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req pullRequest
	if err := dec.Decode(&req); err != nil {
		api.WriteError(w, api.ErrInvalidRequest)
		return
	}

	ctx := r.Context()

	// 1) 记录 Pull 联系与设备自述信息（last_pull_at / last_seen / 版本 / capabilities）。
	if err := h.devices.RecordPull(ctx, dev.ID, device.EnrollInfo{
		AgentVersion: req.AgentVersion,
		MgateVersion: req.MgateVersion,
		DeviceModel:  req.DeviceModel,
		Hostname:     req.Hostname,
		FirmwareInfo: req.FirmwareInfo,
	}, req.Capabilities); err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}

	// 2) 状态上报（来源标记 pull）。
	if len(req.Status) > 0 {
		if err := h.devices.SaveStatus(ctx, dev.ID, string(req.Status), time.Now().UTC(), device.StatusSourcePull); err == nil {
			_ = h.devices.MarkPullStatus(ctx, dev.ID)
			h.record(ctx, model.ActionDevicePullStatus, dev.ID, "Pull 状态上报", ip, ua, requestID, nil)
		}
	}

	// 3) 处理 acks（命令归属校验在 command.Service 内完成）。
	for _, a := range req.Acks {
		if a.CommandID == "" {
			continue
		}
		h.commands.HandleAck(ctx, dev.ID, command.AckInput{
			CommandID: a.CommandID, Accepted: a.Accepted, Message: a.Message,
		}, ip, ua, requestID)
	}

	// 4) 处理 results（单条异常不影响整个 Pull）。
	for _, res := range req.Results {
		if res.CommandID == "" {
			continue
		}
		h.commands.HandleResult(ctx, dev.ID, command.ResultInput{
			CommandID:    res.CommandID,
			Status:       res.Status,
			ExitCode:     res.ExitCode,
			Stdout:       res.Stdout,
			Stderr:       res.Stderr,
			Result:       res.Result,
			ErrorMessage: res.ErrorMessage,
			StartedAt:    res.StartedAt,
			FinishedAt:   res.FinishedAt,
		}, ip, ua, requestID)
	}

	// 5) 领取待投递命令。
	limit := h.resolveLimit(req.MaxCommands)
	commands, err := h.commands.LeaseForPull(ctx, dev.ID, requestID, limit)
	if err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}
	if len(commands) > 0 {
		h.record(ctx, model.ActionDevicePull, dev.ID, "Pull 领取命令", ip, ua, requestID, map[string]any{"count": len(commands)})
	}

	api.WriteSuccess(w, http.StatusOK, map[string]any{
		"server_time":         time.Now().UTC(),
		"next_pull_after_sec": int(h.defaultInterval.Seconds()),
		"commands":            commands,
	})
}

// resolveLimit 把请求的 max_commands 规整到 [1, maxCommands]。
func (h *PullHandlers) resolveLimit(requested int) int {
	if requested <= 0 {
		return 1
	}
	if h.maxCommands > 0 && requested > h.maxCommands {
		return h.maxCommands
	}
	return requested
}

// record 写一条设备相关审计。
func (h *PullHandlers) record(ctx context.Context, action, deviceID, summary, ip, ua, requestID string, metadata map[string]any) {
	h.audit.Record(ctx, audit.Entry{
		ActorType:  model.ActorTypeDevice,
		ActorID:    deviceID,
		Action:     action,
		TargetType: model.TargetTypeDevice,
		TargetID:   deviceID,
		IP:         ip,
		UserAgent:  ua,
		RequestID:  requestID,
		Summary:    summary,
		Metadata:   metadata,
	})
}

// auditAuthFailed 记录 Pull 鉴权失败（绝不记录 token）。
func (h *PullHandlers) auditAuthFailed(ctx context.Context, deviceID, reason, ip, ua, requestID string) {
	h.record(ctx, model.ActionDevicePullAuthFail, deviceID, "Pull 鉴权失败", ip, ua, requestID, map[string]any{"reason": reason})
}
