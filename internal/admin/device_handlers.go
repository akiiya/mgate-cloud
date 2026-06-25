package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/device"
	"mgate-cloud/internal/model"
)

// DeviceHandlers 聚合管理员侧的设备管理 HTTP 处理器。
//
// 与 Handlers 拆分，是为了让认证相关与设备相关各自内聚；二者同包共用中间件。
type DeviceHandlers struct {
	devices *device.Service
	audit   *audit.Service
}

// NewDeviceHandlers 构造设备管理处理器。
func NewDeviceHandlers(deviceService *device.Service, auditService *audit.Service) *DeviceHandlers {
	return &DeviceHandlers{devices: deviceService, audit: auditService}
}

// deviceDTO 是设备对外的稳定 JSON 形态。
//
// 用专门的 DTO（而非直接序列化 model）控制字段名与可见性，避免内部结构变动
// 意外改变 API 契约。可空字段以指针表达，序列化为 null。
type deviceDTO struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Remark               *string    `json:"remark"`
	Status               string     `json:"status"`
	Online               bool       `json:"online"`
	AgentVersion         string     `json:"agent_version"`
	MgateVersion         string     `json:"mgate_version"`
	DeviceModel          string     `json:"device_model"`
	Hostname             string     `json:"hostname"`
	FirmwareInfo         string     `json:"firmware_info"`
	LastSeenAt           *time.Time `json:"last_seen_at"`
	LastEnrolledAt       *time.Time `json:"last_enrolled_at"`
	LastWSConnectedAt    *time.Time `json:"last_ws_connected_at"`
	LastWSDisconnectedAt *time.Time `json:"last_ws_disconnected_at"`
	LastPullAt           *time.Time `json:"last_pull_at"`
	LastPullStatusAt     *time.Time `json:"last_pull_status_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// toDeviceDTO 把领域模型 + 在线状态映射为 DTO。可空字符串统一呈现为空串。
func toDeviceDTO(d model.Device, online bool) deviceDTO {
	return deviceDTO{
		ID:                   d.ID,
		Name:                 d.Name,
		Remark:               d.Remark,
		Status:               d.Status,
		Online:               online,
		AgentVersion:         deref(d.AgentVersion),
		MgateVersion:         deref(d.MgateVersion),
		DeviceModel:          deref(d.DeviceModel),
		Hostname:             deref(d.Hostname),
		FirmwareInfo:         deref(d.FirmwareInfo),
		LastSeenAt:           d.LastSeenAt,
		LastEnrolledAt:       d.LastEnrolledAt,
		LastWSConnectedAt:    d.LastWSConnectedAt,
		LastWSDisconnectedAt: d.LastWSDisconnectedAt,
		LastPullAt:           d.LastPullAt,
		LastPullStatusAt:     d.LastPullStatusAt,
		CreatedAt:            d.CreatedAt,
		UpdatedAt:            d.UpdatedAt,
	}
}

// List 返回设备列表（含在线状态）。
func (h *DeviceHandlers) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.devices.ListDevices(r.Context())
	if err != nil {
		api.WriteError(w, err)
		return
	}

	dtos := make([]deviceDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toDeviceDTO(it.Device, it.Online))
	}
	api.WriteSuccess(w, http.StatusOK, map[string]any{"items": dtos})
}

// createDeviceRequest 是创建设备请求体。
type createDeviceRequest struct {
	Name   string `json:"name"`
	Remark string `json:"remark"`
}

// Create 创建一台 pending 设备。
func (h *DeviceHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var req createDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		api.WriteError(w, api.ErrBadRequest)
		return
	}

	d, err := h.devices.CreateDevice(r.Context(), req.Name, req.Remark)
	if err != nil {
		api.WriteError(w, err)
		return
	}

	admin, _ := AdminFrom(r.Context())
	h.recordDeviceAudit(r, model.ActionDeviceCreate, admin.ID, d.ID, "创建设备", map[string]any{"name": d.Name})

	// 新建设备尚未连接，online 必为 false。
	api.WriteSuccess(w, http.StatusOK, map[string]any{"device": toDeviceDTO(d, false)})
}

// Detail 返回设备详情（含在线状态、最近设备码状态、凭证摘要与最新状态）。
func (h *DeviceHandlers) Detail(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	detail, err := h.devices.GetDeviceDetail(r.Context(), deviceID)
	if err != nil {
		api.WriteError(w, err)
		return
	}

	resp := map[string]any{
		"device": toDeviceDTO(detail.Device, detail.Online),
		"online": detail.Online,
		"pairing": map[string]any{
			"status":     detail.PairingStatus,
			"expires_at": detail.PairingExpiresAt,
		},
		"credential": map[string]any{
			"active_count": detail.ActiveCredentialCount,
		},
		// 默认无状态时这些字段为 null。
		"latest_status":             nil,
		"latest_status_reported_at": nil,
		"latest_status_received_at": nil,
		"latest_status_source":      nil,
	}

	if detail.LatestStatus != nil {
		// 把存库的状态 JSON 文本原样作为 JSON 对象嵌入响应。
		resp["latest_status"] = json.RawMessage(detail.LatestStatus.StatusJSON)
		resp["latest_status_reported_at"] = detail.LatestStatus.ReportedAt
		resp["latest_status_received_at"] = detail.LatestStatus.ReceivedAt
		resp["latest_status_source"] = detail.LatestStatus.Source
	}

	api.WriteSuccess(w, http.StatusOK, resp)
}

// PairingCode 为 pending 设备生成一次性设备码。设备码明文只在此返回一次。
func (h *DeviceHandlers) PairingCode(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	admin, _ := AdminFrom(r.Context())

	code, expiresAt, err := h.devices.GeneratePairingCode(r.Context(), deviceID, admin.ID)
	if err != nil {
		api.WriteError(w, err)
		return
	}

	// 审计只记录"为哪台设备生成、何时过期"，绝不记录设备码/令牌明文。
	h.recordDeviceAudit(r, model.ActionPairingCodeCreate, admin.ID, deviceID, "生成一次性设备码",
		map[string]any{"expires_at": expiresAt})

	api.WriteSuccess(w, http.StatusOK, map[string]any{
		"device_code": code,
		"expires_at":  expiresAt,
	})
}

// Disable 禁用设备（幂等）。
func (h *DeviceHandlers) Disable(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if err := h.devices.DisableDevice(r.Context(), deviceID); err != nil {
		api.WriteError(w, err)
		return
	}
	admin, _ := AdminFrom(r.Context())
	h.recordDeviceAudit(r, model.ActionDeviceDisable, admin.ID, deviceID, "禁用设备", nil)
	api.WriteSuccess(w, http.StatusOK, nil)
}

// Enable 启用设备（依据是否曾绑定回到 enabled 或 pending）。
func (h *DeviceHandlers) Enable(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if err := h.devices.EnableDevice(r.Context(), deviceID); err != nil {
		api.WriteError(w, err)
		return
	}
	admin, _ := AdminFrom(r.Context())
	h.recordDeviceAudit(r, model.ActionDeviceEnable, admin.ID, deviceID, "启用设备", nil)
	api.WriteSuccess(w, http.StatusOK, nil)
}

// recordDeviceAudit 统一记录设备相关审计，集中填充 ip/ua/request_id，减少重复。
func (h *DeviceHandlers) recordDeviceAudit(r *http.Request, action, adminID, deviceID, summary string, metadata map[string]any) {
	h.audit.Record(r.Context(), audit.Entry{
		ActorType:  model.ActorTypeAdmin,
		ActorID:    adminID,
		Action:     action,
		TargetType: model.TargetTypeDevice,
		TargetID:   deviceID,
		IP:         audit.ClientIP(r),
		UserAgent:  r.UserAgent(),
		RequestID:  audit.RequestIDFrom(r.Context()),
		Summary:    summary,
		Metadata:   metadata,
	})
}

// deref 安全解引用可空字符串指针，nil 返回空串。
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
