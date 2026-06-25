package device

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/db"
	"mgate-cloud/internal/model"
	"mgate-cloud/internal/util"
)

// 状态来源标识，写入 device_latest_status / snapshots 的 source 列。
const (
	StatusSourceWS   = "ws"
	StatusSourcePull = "pull"
)

// AuthenticateDevice 校验 WebSocket 接入凭据：设备存在、enabled、有匹配的 active 凭证。
//
// 安全要点：
//   - 设备不存在 / 无有效凭证 / token 不匹配 → 统一返回 ErrUnauthorized，不泄露差异。
//   - disabled 设备返回 ErrDeviceDisabled（403）；pending/deleted 视为未授权（401）。
//   - token 比较使用恒定时间比较，避免计时侧信道。
func (s *Service) AuthenticateDevice(ctx context.Context, deviceID, token string) (model.Device, error) {
	d, err := s.devices.GetByID(ctx, s.db, deviceID)
	if errors.Is(err, errDeviceNotFound) {
		return model.Device{}, api.ErrUnauthorized
	}
	if err != nil {
		return model.Device{}, err
	}

	switch d.Status {
	case model.DeviceStatusEnabled:
		// 继续校验凭证。
	case model.DeviceStatusDisabled:
		return model.Device{}, api.ErrDeviceDisabled
	default:
		// pending / deleted：尚未绑定或已删除，不允许连接。
		return model.Device{}, api.ErrUnauthorized
	}

	cred, err := s.creds.FindActiveByDevice(ctx, s.db, deviceID)
	if errors.Is(err, errCredentialNotFound) {
		return model.Device{}, api.ErrUnauthorized
	}
	if err != nil {
		return model.Device{}, err
	}

	// 恒定时间比较 token 哈希。
	if !util.ConstantTimeEqualString(cred.TokenHash, util.HashTokenHex(token)) {
		return model.Device{}, api.ErrUnauthorized
	}
	return d, nil
}

// EnsureCommandable 校验设备是否允许下发命令。
//
// Phase 5 起【不再要求在线】：enabled 设备无论 online/offline 都可创建命令；
// 离线命令保持 pending，等待 WS 重连或 Pull 领取。
// 设备不存在/已删除 → device_not_found；pending/disabled → device_not_ready。
func (s *Service) EnsureCommandable(ctx context.Context, deviceID string) (model.Device, error) {
	d, err := s.devices.GetByID(ctx, s.db, deviceID)
	if errors.Is(err, errDeviceNotFound) {
		return model.Device{}, api.ErrDeviceNotFound
	}
	if err != nil {
		return model.Device{}, err
	}
	if d.Status == model.DeviceStatusDeleted {
		return model.Device{}, api.ErrDeviceNotFound
	}
	if d.Status != model.DeviceStatusEnabled {
		return model.Device{}, api.ErrDeviceNotReady
	}
	return d, nil
}

// IsOnline 报告设备是否当前 WS 在线（用于命令服务决定优先投递通道）。
func (s *Service) IsOnline(deviceID string) bool {
	return s.isOnline(deviceID)
}

// RecordPull 处理一次 Pull 的设备自述信息上报，并记录 last_pull_at。
func (s *Service) RecordPull(ctx context.Context, deviceID string, info EnrollInfo, capabilities []string) error {
	capabilitiesJSON := ""
	if len(capabilities) > 0 {
		if raw, err := json.Marshal(capabilities); err == nil {
			capabilitiesJSON = string(raw)
		}
	}
	now := s.clock.Now()
	if err := s.devices.ApplyHello(ctx, s.db, deviceID, info, capabilitiesJSON, now); err != nil {
		return err
	}
	return s.devices.SetLastPull(ctx, s.db, deviceID, now)
}

// MarkPullStatus 记录最近一次通过 Pull 上报状态的时间。
func (s *Service) MarkPullStatus(ctx context.Context, deviceID string) error {
	return s.devices.SetLastPullStatus(ctx, s.db, deviceID, s.clock.Now())
}

// MarkWSConnected 记录设备 WebSocket 建连。
func (s *Service) MarkWSConnected(ctx context.Context, deviceID string) error {
	return s.devices.SetWSConnected(ctx, s.db, deviceID, s.clock.Now())
}

// MarkWSDisconnected 记录设备 WebSocket 断开。
func (s *Service) MarkWSDisconnected(ctx context.Context, deviceID string) error {
	return s.devices.SetWSDisconnected(ctx, s.db, deviceID, s.clock.Now())
}

// ApplyHello 依据 agent.hello 更新设备自述信息与能力声明。
//
// 注意：capabilities 仅作记录，本阶段【不】据此向设备下发任何命令。
func (s *Service) ApplyHello(ctx context.Context, deviceID string, info EnrollInfo, capabilities []string) error {
	capabilitiesJSON := ""
	if len(capabilities) > 0 {
		if raw, err := json.Marshal(capabilities); err == nil {
			capabilitiesJSON = string(raw)
		}
	}
	return s.devices.ApplyHello(ctx, s.db, deviceID, info, capabilitiesJSON, s.clock.Now())
}

// TouchHeartbeat 处理心跳：刷新设备最近活跃时间。
func (s *Service) TouchHeartbeat(ctx context.Context, deviceID string) error {
	return s.devices.TouchLastSeen(ctx, s.db, deviceID, s.clock.Now())
}

// SaveStatus 保存设备上报的状态：覆盖最新状态、追加历史快照、刷新活跃时间。
//
// 三个写入放在同一事务，保证一致性。source 标识来源（ws / pull）。
func (s *Service) SaveStatus(ctx context.Context, deviceID, statusJSON string, reportedAt time.Time, source string) error {
	now := s.clock.Now()
	rec := StatusRecord{
		DeviceID:   deviceID,
		StatusJSON: statusJSON,
		ReportedAt: reportedAt,
		ReceivedAt: now,
		Source:     source,
	}
	return db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := s.status.UpsertLatest(ctx, tx, rec); err != nil {
			return err
		}
		if err := s.status.InsertSnapshot(ctx, tx, rec); err != nil {
			return err
		}
		return s.devices.TouchLastSeen(ctx, tx, deviceID, now)
	})
}
