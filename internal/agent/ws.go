package agent

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/command"
	"mgate-cloud/internal/device"
	"mgate-cloud/internal/hub"
	"mgate-cloud/internal/model"
	"mgate-cloud/internal/util"
)

// 鉴权 header 名。
const (
	headerDeviceID = "X-Mgate-Device-ID"
	headerAuth     = "Authorization"
	bearerPrefix   = "Bearer "
)

// WSHandlers 处理 agent WebSocket 接入与消息分发。
type WSHandlers struct {
	devices           *device.Service
	commands          *command.Service
	audit             *audit.Service
	hub               *hub.Hub
	heartbeatInterval time.Duration
	offlineAfter      time.Duration
	maxMessageBytes   int64
}

// NewWSHandlers 构造 WebSocket 处理器。
func NewWSHandlers(deviceService *device.Service, commandService *command.Service, auditService *audit.Service, h *hub.Hub, heartbeatInterval, offlineAfter time.Duration, maxMessageBytes int64) *WSHandlers {
	return &WSHandlers{
		devices:           deviceService,
		commands:          commandService,
		audit:             auditService,
		hub:               h,
		heartbeatInterval: heartbeatInterval,
		offlineAfter:      offlineAfter,
		maxMessageBytes:   maxMessageBytes,
	}
}

// ServeWS 处理 GET /api/agent/ws。
//
// 鉴权在 WebSocket 升级【之前】完成：失败直接返回 401/403，绝不升级连接。
func (h *WSHandlers) ServeWS(w http.ResponseWriter, r *http.Request) {
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

	// 升级为 WebSocket。OriginPatterns 放开：鉴权基于 bearer token 而非 cookie，
	// 不存在浏览器跨站凭据被滥用的问题；agent 通常也不携带 Origin。
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		// 升级失败时响应已被接管，无法再写 JSON，仅记录日志。
		log.Printf("agent: WebSocket 升级失败 device_id=%s: %v", deviceID, err)
		return
	}
	c.SetReadLimit(h.maxMessageBytes)

	h.serve(c, dev, ip, ua, requestID)
}

// serve 管理单条连接的完整生命周期：注册→连接审计→读循环→断开清理。
func (h *WSHandlers) serve(c *websocket.Conn, dev model.Device, ip, ua, requestID string) {
	now := time.Now().UTC()
	connID := util.NewID()
	conn := hub.NewConnection(dev.ID, connID, ip, ua, c, now)

	// connCtx 与 HTTP 请求生命周期解耦：用于连接期间的 DB/审计写入，
	// 在所有清理完成后才取消（defer 顺序：cleanup 先于 cancel 执行）。
	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 同设备新连接接入：取代并关闭旧连接。
	if replaced := h.hub.Register(conn); replaced != nil {
		replaced.Close(websocket.StatusPolicyViolation, "replaced by new connection")
	}

	_ = h.devices.MarkWSConnected(connCtx, dev.ID)
	h.record(connCtx, model.ActionDeviceWSConnect, dev.ID, "设备 WebSocket 连接", ip, ua, requestID, nil)

	defer func() {
		// 仅当当前登记的仍是本连接时才注销（connID 守卫，避免误删接管者）。
		h.hub.Unregister(dev.ID, connID)
		conn.Close(websocket.StatusNormalClosure, "")
		_ = h.devices.MarkWSDisconnected(connCtx, dev.ID)
		h.record(connCtx, model.ActionDeviceWSDisconnect, dev.ID, "设备 WebSocket 断开", ip, ua, requestID, nil)
	}()

	for {
		// 每条消息设读超时为 offlineAfter：长时间无消息即判定失活并断开。
		readCtx, rcancel := context.WithTimeout(connCtx, h.offlineAfter)
		data, err := conn.Read(readCtx)
		rcancel()
		if err != nil {
			return // 连接出错/超时/对端关闭 → 退出，触发清理
		}
		h.processMessage(connCtx, conn, dev, data, ip, ua, requestID)
	}
}

// processMessage 解析并分发单条消息。任何异常都被收敛，绝不让连接因单条消息 panic。
func (h *WSHandlers) processMessage(ctx context.Context, conn *hub.Connection, dev model.Device, data []byte, ip, ua, requestID string) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("agent: WebSocket 消息处理 panic device_id=%s: %v", dev.ID, rec)
		}
	}()

	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		h.sendError(ctx, conn, dev.ID, "invalid_envelope", "无法解析消息")
		return
	}
	if env.V != envelopeVersion || env.ID == "" || env.Type == "" || env.TS.IsZero() {
		h.sendError(ctx, conn, dev.ID, "invalid_envelope", "信封字段缺失或版本不符")
		return
	}
	// device_id 必须与认证通过的设备一致，否则视为协议违规并断开。
	if env.DeviceID != dev.ID {
		h.sendError(ctx, conn, dev.ID, "device_mismatch", "device_id 不匹配")
		conn.Close(websocket.StatusPolicyViolation, "device_id mismatch")
		return
	}

	switch env.Type {
	case typeAgentHello:
		h.handleHello(ctx, conn, dev, env, ip, ua, requestID)
	case typeAgentHeartbeat:
		h.handleHeartbeat(ctx, conn, dev)
	case typeAgentStatus:
		h.handleStatus(ctx, conn, dev, env, ip, ua, requestID)
	case typeCommandAck:
		h.handleCommandAck(ctx, conn, dev, env, ip, ua, requestID)
	case typeCommandResult:
		h.handleCommandResult(ctx, conn, dev, env, ip, ua, requestID)
	default:
		// 未知类型：回 error 信封并保持连接（已文档化的行为）。
		h.sendError(ctx, conn, dev.ID, "unknown_type", "不支持的消息类型")
	}
}

// handleHello 处理 agent.hello：更新设备信息与能力声明，回 server.hello。
func (h *WSHandlers) handleHello(ctx context.Context, conn *hub.Connection, dev model.Device, env Envelope, ip, ua, requestID string) {
	var p helloPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		h.sendError(ctx, conn, dev.ID, "invalid_payload", "hello 载荷无效")
		return
	}

	info := device.EnrollInfo{
		AgentVersion: p.AgentVersion,
		MgateVersion: p.MgateVersion,
		DeviceModel:  p.DeviceModel,
		Hostname:     p.Hostname,
		FirmwareInfo: p.FirmwareInfo,
	}
	if err := h.devices.ApplyHello(ctx, dev.ID, info, p.Capabilities); err != nil {
		log.Printf("agent: 应用 hello 失败 device_id=%s: %v", dev.ID, err)
	}

	conn.MarkHeartbeat(time.Now().UTC())
	h.record(ctx, model.ActionDeviceHello, dev.ID, "设备上线 hello", ip, ua, requestID, map[string]any{
		"agent_version":      p.AgentVersion,
		"mgate_version":      p.MgateVersion,
		"hostname":           p.Hostname,
		"capabilities_count": len(p.Capabilities),
	})

	h.reply(ctx, conn, typeServerHello, dev.ID, serverHelloPayload{
		ServerTime:           time.Now().UTC(),
		HeartbeatIntervalSec: int(h.heartbeatInterval.Seconds()),
	})

	// WS 重连后尝试投递该设备的待处理命令（与 Pull 共用 lease 机制，不会重复投递）。
	h.commands.DispatchPending(ctx, dev.ID)
}

// handleHeartbeat 处理 agent.heartbeat：更新内存心跳与 DB last_seen_at，回 server.pong。
// 心跳默认不写审计，避免日志刷屏。
func (h *WSHandlers) handleHeartbeat(ctx context.Context, conn *hub.Connection, dev model.Device) {
	conn.MarkHeartbeat(time.Now().UTC())
	if err := h.devices.TouchHeartbeat(ctx, dev.ID); err != nil {
		log.Printf("agent: 更新心跳失败 device_id=%s: %v", dev.ID, err)
	}
	h.reply(ctx, conn, typeServerPong, dev.ID, map[string]any{"server_time": time.Now().UTC()})
}

// handleStatus 处理 agent.status：保存最新状态与历史快照，写审计（仅摘要，不存大 JSON）。
func (h *WSHandlers) handleStatus(ctx context.Context, conn *hub.Connection, dev model.Device, env Envelope, ip, ua, requestID string) {
	if len(env.Payload) == 0 {
		h.sendError(ctx, conn, dev.ID, "invalid_payload", "status 载荷为空")
		return
	}
	statusJSON := string(env.Payload)
	if err := h.devices.SaveStatus(ctx, dev.ID, statusJSON, env.TS, device.StatusSourceWS); err != nil {
		log.Printf("agent: 保存状态失败 device_id=%s: %v", dev.ID, err)
		h.sendError(ctx, conn, dev.ID, "internal_error", "保存状态失败")
		return
	}

	conn.MarkHeartbeat(time.Now().UTC())
	// 审计仅记录顶层字段名摘要，绝不保存完整大 JSON。
	h.record(ctx, model.ActionDeviceStatus, dev.ID, "设备状态上报", ip, ua, requestID, map[string]any{
		"sections": topLevelKeys(env.Payload),
	})
}

// handleCommandAck 处理 agent 的 command.ack。
func (h *WSHandlers) handleCommandAck(ctx context.Context, conn *hub.Connection, dev model.Device, env Envelope, ip, ua, requestID string) {
	var p ackPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil || p.CommandID == "" {
		h.sendError(ctx, conn, dev.ID, "invalid_payload", "ack 载荷无效")
		return
	}
	// 命令归属校验在 service 内完成（command 必须属于本连接 device_id）。
	h.commands.HandleAck(ctx, dev.ID, command.AckInput{
		CommandID: p.CommandID,
		Accepted:  p.Accepted,
		Message:   p.Message,
	}, ip, ua, requestID)
}

// handleCommandResult 处理 agent 的 command.result。
func (h *WSHandlers) handleCommandResult(ctx context.Context, conn *hub.Connection, dev model.Device, env Envelope, ip, ua, requestID string) {
	var p resultPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil || p.CommandID == "" {
		h.sendError(ctx, conn, dev.ID, "invalid_payload", "result 载荷无效")
		return
	}
	h.commands.HandleResult(ctx, dev.ID, command.ResultInput{
		CommandID:    p.CommandID,
		Status:       p.Status,
		ExitCode:     p.ExitCode,
		Stdout:       p.Stdout,
		Stderr:       p.Stderr,
		Result:       p.Result,
		ErrorMessage: p.ErrorMessage,
		StartedAt:    p.StartedAt,
		FinishedAt:   p.FinishedAt,
	}, ip, ua, requestID)
}

// reply 发送一条 cloud→agent 信封。
func (h *WSHandlers) reply(ctx context.Context, conn *hub.Connection, msgType, deviceID string, payload any) {
	raw, err := newEnvelope(time.Now().UTC(), msgType, deviceID, payload)
	if err != nil {
		log.Printf("agent: 构造 %s 信封失败: %v", msgType, err)
		return
	}
	if err := conn.WriteJSON(ctx, raw); err != nil {
		log.Printf("agent: 发送 %s 失败 device_id=%s: %v", msgType, deviceID, err)
	}
}

// sendError 回一条 error 信封。
func (h *WSHandlers) sendError(ctx context.Context, conn *hub.Connection, deviceID, code, message string) {
	h.reply(ctx, conn, typeError, deviceID, errorPayload{Code: code, Message: message})
}

// record 写一条设备相关审计，集中填充公共字段。
func (h *WSHandlers) record(ctx context.Context, action, deviceID, summary, ip, ua, requestID string, metadata map[string]any) {
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

// auditAuthFailed 记录鉴权失败（绝不记录 token）。
func (h *WSHandlers) auditAuthFailed(ctx context.Context, deviceID, reason, ip, ua, requestID string) {
	h.record(ctx, model.ActionDeviceWSAuthFailed, deviceID, "设备 WebSocket 鉴权失败", ip, ua, requestID, map[string]any{
		"reason": reason,
	})
}

// bearerToken 从 Authorization 头解析 Bearer 令牌，无效返回空串。
func bearerToken(header string) string {
	if len(header) > len(bearerPrefix) && strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return strings.TrimSpace(header[len(bearerPrefix):])
	}
	return ""
}

// topLevelKeys 返回 JSON 对象的顶层键名（用于审计摘要），解析失败返回 nil。
func topLevelKeys(raw json.RawMessage) []string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
