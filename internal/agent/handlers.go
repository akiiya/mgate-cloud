// Package agent 提供面向设备 agent 的公开 HTTP 接口。
//
// Phase 2 仅实现 enroll（设备绑定）。本包【不】包含任何设备控制能力：
// 没有 WebSocket、没有 Pull、没有命令队列、没有远程 shell。
package agent

import (
	"encoding/json"
	"net/http"
	"strings"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/device"
	"mgate-cloud/internal/model"
)

// maxEnrollBody 限制 enroll 请求体大小，防止超大请求消耗资源。
const maxEnrollBody = 16 << 10 // 16 KiB

// Handlers 聚合 agent 公开接口处理器。
type Handlers struct {
	devices *device.Service
	audit   *audit.Service
}

// NewHandlers 构造 agent 处理器。
func NewHandlers(deviceService *device.Service, auditService *audit.Service) *Handlers {
	return &Handlers{devices: deviceService, audit: auditService}
}

// enrollRequest 是 enroll 请求体。device_info 为嵌套对象。
type enrollRequest struct {
	DeviceCode   string `json:"device_code"`
	AgentVersion string `json:"agent_version"`
	DeviceInfo   struct {
		Hostname     string `json:"hostname"`
		Model        string `json:"model"`
		MgateVersion string `json:"mgate_version"`
		FirmwareInfo string `json:"firmware_info"`
	} `json:"device_info"`
}

// Enroll 处理设备绑定。
//
// 该接口面向公网设备，因此【无需】管理员 session、【不】走 CSRF——
// 其信任完全建立在"持有有效一次性设备码"之上，由 device.Service 严格校验。
func (h *Handlers) Enroll(w http.ResponseWriter, r *http.Request) {
	// 限制请求体大小，并严格解析（拒绝未知字段），降低被滥用的面。
	r.Body = http.MaxBytesReader(w, r.Body, maxEnrollBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req enrollRequest
	if err := dec.Decode(&req); err != nil {
		h.recordFailure(r, "", "invalid_request", req.DeviceInfo.Hostname, req.AgentVersion)
		api.WriteError(w, api.ErrInvalidRequest)
		return
	}
	if strings.TrimSpace(req.DeviceCode) == "" {
		h.recordFailure(r, "", "invalid_request", req.DeviceInfo.Hostname, req.AgentVersion)
		api.WriteError(w, api.ErrInvalidRequest)
		return
	}

	info := device.EnrollInfo{
		AgentVersion: req.AgentVersion,
		MgateVersion: req.DeviceInfo.MgateVersion,
		DeviceModel:  req.DeviceInfo.Model,
		Hostname:     req.DeviceInfo.Hostname,
		FirmwareInfo: req.DeviceInfo.FirmwareInfo,
	}

	result, err := h.devices.Enroll(r.Context(), req.DeviceCode, info)
	if err != nil {
		// 审计失败：仅记录失败原因 code 与设备自述信息，绝不记录设备码/令牌。
		h.recordFailure(r, "", errCode(err), req.DeviceInfo.Hostname, req.AgentVersion)
		api.WriteError(w, err)
		return
	}

	// 审计成功：记录设备 id 与自述信息，绝不记录 device_token 明文。
	h.audit.Record(r.Context(), audit.Entry{
		ActorType:  model.ActorTypeDevice,
		ActorID:    result.DeviceID,
		Action:     model.ActionDeviceEnrollOK,
		TargetType: model.TargetTypeDevice,
		TargetID:   result.DeviceID,
		IP:         audit.ClientIP(r),
		UserAgent:  r.UserAgent(),
		RequestID:  audit.RequestIDFrom(r.Context()),
		Summary:    "设备绑定成功",
		Metadata: map[string]any{
			"hostname":      req.DeviceInfo.Hostname,
			"agent_version": req.AgentVersion,
			"mgate_version": req.DeviceInfo.MgateVersion,
		},
	})

	api.WriteSuccess(w, http.StatusOK, map[string]any{
		"device_id":    result.DeviceID,
		"device_token": result.DeviceToken, // 仅此一次返回明文
		"gateway":      result.Gateway,
		"ws_url":       result.WSURL,
		"pull_url":     result.PullURL,
	})
}

// recordFailure 记录一次 enroll 失败审计。
func (h *Handlers) recordFailure(r *http.Request, deviceID, reason, hostname, agentVersion string) {
	h.audit.Record(r.Context(), audit.Entry{
		ActorType:  model.ActorTypeDevice,
		ActorID:    deviceID,
		Action:     model.ActionDeviceEnrollFail,
		TargetType: model.TargetTypeDevice,
		TargetID:   deviceID,
		IP:         audit.ClientIP(r),
		UserAgent:  r.UserAgent(),
		RequestID:  audit.RequestIDFrom(r.Context()),
		Summary:    "设备绑定失败",
		Metadata: map[string]any{
			"reason":        reason,
			"hostname":      hostname,
			"agent_version": agentVersion,
		},
	})
}

// errCode 提取错误的稳定 code 用于审计；非 API 错误归为 internal_error。
func errCode(err error) string {
	if apiErr, ok := err.(*api.Error); ok {
		return apiErr.Code
	}
	return "internal_error"
}
