package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/command"
	"mgate-cloud/internal/model"
)

// CommandHandlers 聚合管理员侧的命令相关 HTTP 处理器。
type CommandHandlers struct {
	commands *command.Service
}

// NewCommandHandlers 构造命令处理器。审计在 command.Service 内完成。
func NewCommandHandlers(commandService *command.Service) *CommandHandlers {
	return &CommandHandlers{commands: commandService}
}

// commandDTO 是命令对外的稳定 JSON 形态。params 原样作为 JSON 对象输出。
type commandDTO struct {
	ID          string          `json:"id"`
	DeviceID    string          `json:"device_id"`
	DeviceName  string          `json:"device_name,omitempty"`
	Action      string          `json:"action"`
	Params      json.RawMessage `json:"params"`
	Status      string          `json:"status"`
	TimeoutSec  int             `json:"timeout_sec"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	LeasedBy    *string         `json:"leased_by"`
	LeaseUntil  *time.Time      `json:"lease_until"`
	ExpiresAt   *time.Time      `json:"expires_at"`
	LastError   *string         `json:"last_error"`
	CreatedAt   time.Time       `json:"created_at"`
	SentAt      *time.Time      `json:"sent_at"`
	AckedAt     *time.Time      `json:"acked_at"`
	StartedAt   *time.Time      `json:"started_at"`
	FinishedAt  *time.Time      `json:"finished_at"`
}

func toCommandDTO(c model.Command, deviceName string) commandDTO {
	return commandDTO{
		ID:          c.ID,
		DeviceID:    c.DeviceID,
		DeviceName:  deviceName,
		Action:      c.Action,
		Params:      json.RawMessage(c.ParamsJSON),
		Status:      c.Status,
		TimeoutSec:  c.TimeoutSec,
		Attempts:    c.Attempts,
		MaxAttempts: c.MaxAttempts,
		LeasedBy:    c.LeasedBy,
		LeaseUntil:  c.LeaseUntil,
		ExpiresAt:   c.ExpiresAt,
		LastError:   c.LastError,
		CreatedAt:   c.CreatedAt,
		SentAt:      c.SentAt,
		AckedAt:     c.AckedAt,
		StartedAt:   c.StartedAt,
		FinishedAt:  c.FinishedAt,
	}
}

// resultDTO 是命令结果对外形态。result 原样作为 JSON 输出。
type resultDTO struct {
	Status       string          `json:"status"`
	ExitCode     *int            `json:"exit_code"`
	Stdout       string          `json:"stdout"`
	Stderr       string          `json:"stderr"`
	Result       json.RawMessage `json:"result"`
	ErrorMessage string          `json:"error_message"`
	Truncated    bool            `json:"truncated"`
	StartedAt    *time.Time      `json:"started_at"`
	FinishedAt   *time.Time      `json:"finished_at"`
	ReceivedAt   time.Time       `json:"received_at"`
}

func toResultDTO(r model.CommandResult) resultDTO {
	return resultDTO{
		Status:       r.Status,
		ExitCode:     r.ExitCode,
		Stdout:       derefStr(r.Stdout),
		Stderr:       derefStr(r.Stderr),
		Result:       rawOrNull(r.ResultJSON),
		ErrorMessage: derefStr(r.ErrorMessage),
		Truncated:    r.Truncated,
		StartedAt:    r.StartedAt,
		FinishedAt:   r.FinishedAt,
		ReceivedAt:   r.ReceivedAt,
	}
}

// List 返回命令列表，支持 device_id / status / limit 过滤。
func (h *CommandHandlers) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	items, err := h.commands.ListCommands(r.Context(), command.ListFilter{
		DeviceID: q.Get("device_id"),
		Status:   q.Get("status"),
		Limit:    limit,
	})
	if err != nil {
		api.WriteError(w, err)
		return
	}
	dtos := make([]commandDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toCommandDTO(it.Command, it.DeviceName))
	}
	api.WriteSuccess(w, http.StatusOK, map[string]any{"items": dtos})
}

// Detail 返回命令详情及其结果。
func (h *CommandHandlers) Detail(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("command_id")
	cmd, result, err := h.commands.GetCommandDetail(r.Context(), commandID)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	resp := map[string]any{
		"command": toCommandDTO(cmd, ""),
		"result":  nil,
	}
	if result != nil {
		resp["result"] = toResultDTO(*result)
	}
	api.WriteSuccess(w, http.StatusOK, resp)
}

// createCommandRequest 是创建命令的请求体。params 为原始 JSON 对象。
type createCommandRequest struct {
	Action     string          `json:"action"`
	Params     json.RawMessage `json:"params"`
	TimeoutSec int             `json:"timeout_sec"`
}

// Create 为设备创建命令（仅在线 enabled 设备）。
func (h *CommandHandlers) Create(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	var req createCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrBadRequest)
		return
	}
	admin, _ := AdminFrom(r.Context())

	cmd, hint, err := h.commands.CreateCommand(r.Context(), command.CreateInput{
		DeviceID:   deviceID,
		AdminID:    admin.ID,
		Action:     req.Action,
		RawParams:  req.Params,
		TimeoutSec: req.TimeoutSec,
		IP:         audit.ClientIP(r),
		UserAgent:  r.UserAgent(),
		RequestID:  audit.RequestIDFrom(r.Context()),
	})
	if err != nil {
		api.WriteError(w, err)
		return
	}
	api.WriteSuccess(w, http.StatusOK, map[string]any{
		"command":       toCommandDTO(cmd, ""),
		"delivery_hint": hint,
	})
}

// Cancel 取消命令（cloud 侧标记）。
func (h *CommandHandlers) Cancel(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("command_id")
	admin, _ := AdminFrom(r.Context())
	if err := h.commands.CancelCommand(r.Context(), commandID, admin.ID,
		audit.ClientIP(r), r.UserAgent(), audit.RequestIDFrom(r.Context())); err != nil {
		api.WriteError(w, err)
		return
	}
	api.WriteSuccess(w, http.StatusOK, nil)
}

// derefStr 安全解引用字符串指针。
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// rawOrNull 把存库的 JSON 文本指针转为 RawMessage；nil 输出 JSON null。
func rawOrNull(s *string) json.RawMessage {
	if s == nil || *s == "" {
		return json.RawMessage("null")
	}
	return json.RawMessage(*s)
}
