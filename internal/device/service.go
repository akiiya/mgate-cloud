package device

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/db"
	"mgate-cloud/internal/model"
	"mgate-cloud/internal/util"
)

// 令牌前缀：便于人工辨识用途，不参与安全性。
const (
	pairingTokenPrefix = "mgpair_"
	deviceTokenPrefix  = "mgdt_"
)

// 设备码生成的载荷版本号。
const codePayloadVersion = 1

// 设备码摘要状态（用于设备详情展示，非数据库枚举）。
const (
	PairingSummaryNone    = "none"    // 从未生成
	PairingSummaryActive  = "active"  // 存在未使用且未过期的设备码
	PairingSummaryUsed    = "used"    // 最近一次已被使用
	PairingSummaryExpired = "expired" // 最近一次已过期且未使用
)

// Settings 是设备服务运行所需的配置快照，由上层从全局 config 注入。
type Settings struct {
	PairingTTL        time.Duration
	DeviceTokenBytes  int
	PairingTokenBytes int
	Gateway           string // 对外网关地址（= BaseURL）
	WSURL             string // 预告给 agent 的 WS 地址
	PullURL           string // 预告给 agent 的 Pull 地址（Phase 3 不实现该端点）
}

// Presence 抽象"设备是否在线"的查询，由 Hub（进程内 WebSocket 连接）实现。
//
// 之所以用接口注入而非直接依赖 hub 包：避免 device→hub 的硬依赖与潜在循环，
// 也让设备服务在无连接层的单元测试中可独立运行（presence 为 nil 即一律离线）。
type Presence interface {
	IsOnline(deviceID string) bool
}

// Service 编排设备身份与状态的全部用例。
type Service struct {
	db       *sql.DB
	clock    util.Clock
	devices  *DeviceStore
	creds    *CredentialStore
	pairings *PairingStore
	status   *StatusStore
	codec    *Codec
	presence Presence
	cfg      Settings
}

// NewService 构造设备服务。presence 可为 nil（视为始终离线，便于测试）。
func NewService(
	database *sql.DB, clock util.Clock,
	devices *DeviceStore, creds *CredentialStore, pairings *PairingStore, status *StatusStore,
	codec *Codec, presence Presence, cfg Settings,
) *Service {
	return &Service{
		db: database, clock: clock,
		devices: devices, creds: creds, pairings: pairings, status: status,
		codec: codec, presence: presence, cfg: cfg,
	}
}

// isOnline 安全查询在线状态：presence 未注入时一律视为离线。
func (s *Service) isOnline(deviceID string) bool {
	if s.presence == nil {
		return false
	}
	return s.presence.IsOnline(deviceID)
}

// CreateDevice 创建一台 pending 设备。
func (s *Service) CreateDevice(ctx context.Context, name, remark string) (model.Device, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Device{}, api.ErrBadRequest
	}

	now := s.clock.Now()
	d := model.Device{
		ID:        util.NewID(),
		Name:      name,
		Remark:    optionalString(remark),
		Status:    model.DeviceStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.devices.Insert(ctx, s.db, d); err != nil {
		return model.Device{}, err
	}
	return d, nil
}

// DeviceListItem 是列表项：设备本体 + 在线状态。
type DeviceListItem struct {
	Device model.Device
	Online bool
}

// ListDevices 返回设备列表，并附带各设备的在线状态。
func (s *Service) ListDevices(ctx context.Context) ([]DeviceListItem, error) {
	devices, err := s.devices.List(ctx, s.db)
	if err != nil {
		return nil, err
	}
	items := make([]DeviceListItem, 0, len(devices))
	for _, d := range devices {
		items = append(items, DeviceListItem{Device: d, Online: s.isOnline(d.ID)})
	}
	return items, nil
}

// DeviceDetail 是设备详情视图：设备本体 + 在线状态 + 最近设备码 + 凭证摘要 + 最新状态。
type DeviceDetail struct {
	Device                model.Device
	Online                bool
	PairingStatus         string
	PairingExpiresAt      *time.Time
	ActiveCredentialCount int
	LatestStatus          *LatestStatus
}

// GetDeviceDetail 组装设备详情。
func (s *Service) GetDeviceDetail(ctx context.Context, deviceID string) (DeviceDetail, error) {
	d, err := s.devices.GetByID(ctx, s.db, deviceID)
	if err != nil {
		return DeviceDetail{}, mapDeviceErr(err)
	}

	detail := DeviceDetail{Device: d, Online: s.isOnline(d.ID), PairingStatus: PairingSummaryNone}

	if pc, found, err := s.pairings.LatestByDevice(ctx, s.db, deviceID); err != nil {
		return DeviceDetail{}, err
	} else if found {
		detail.PairingStatus = pairingSummary(pc, s.clock.Now())
		detail.PairingExpiresAt = &pc.ExpiresAt
	}

	count, err := s.creds.CountActiveByDevice(ctx, s.db, deviceID)
	if err != nil {
		return DeviceDetail{}, err
	}
	detail.ActiveCredentialCount = count

	if ls, found, err := s.status.GetLatest(ctx, s.db, deviceID); err != nil {
		return DeviceDetail{}, err
	} else if found {
		detail.LatestStatus = &ls
	}

	return detail, nil
}

// GeneratePairingCode 为 pending 设备生成一次性设备码，返回设备码明文与过期时间。
//
// 仅 pending 设备可生成；enabled/disabled 一律拒绝。明文设备码只在此返回一次。
func (s *Service) GeneratePairingCode(ctx context.Context, deviceID, adminID string) (code string, expiresAt time.Time, err error) {
	d, err := s.devices.GetByID(ctx, s.db, deviceID)
	if err != nil {
		return "", time.Time{}, mapDeviceErr(err)
	}
	if !d.IsPending() {
		return "", time.Time{}, api.ErrPairingNotAllowed
	}

	pairingToken, err := util.RandomToken(s.cfg.PairingTokenBytes)
	if err != nil {
		return "", time.Time{}, err
	}
	pairingToken = pairingTokenPrefix + pairingToken

	now := s.clock.Now()
	expiresAt = now.Add(s.cfg.PairingTTL)

	code, err = s.codec.Encode(codePayload{
		V:            codePayloadVersion,
		Gateway:      s.cfg.Gateway,
		PairingToken: pairingToken,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		return "", time.Time{}, err
	}

	pc := model.PairingCode{
		ID:               util.NewID(),
		DeviceID:         deviceID,
		CodeHash:         util.HashTokenHex(pairingToken), // 仅存哈希
		GatewayURL:       s.cfg.Gateway,
		ExpiresAt:        expiresAt,
		CreatedByAdminID: adminID,
		CreatedAt:        now,
	}
	if err := s.pairings.Insert(ctx, s.db, pc); err != nil {
		return "", time.Time{}, err
	}
	return code, expiresAt, nil
}

// DisableDevice 禁用设备；已禁用则幂等成功。不删除凭证。
func (s *Service) DisableDevice(ctx context.Context, deviceID string) error {
	d, err := s.devices.GetByID(ctx, s.db, deviceID)
	if err != nil {
		return mapDeviceErr(err)
	}
	if d.IsDisabled() {
		return nil // 幂等
	}
	return s.devices.UpdateStatus(ctx, s.db, deviceID, model.DeviceStatusDisabled, s.clock.Now())
}

// EnableDevice 启用设备。
//
// 目标状态取决于是否曾经 enroll：有 active 凭证→enabled；从未绑定→pending。
func (s *Service) EnableDevice(ctx context.Context, deviceID string) error {
	// 仅需确认设备存在；具体目标状态由是否有 active 凭证决定。
	if _, err := s.devices.GetByID(ctx, s.db, deviceID); err != nil {
		return mapDeviceErr(err)
	}

	count, err := s.creds.CountActiveByDevice(ctx, s.db, deviceID)
	if err != nil {
		return err
	}
	target := model.DeviceStatusPending
	if count > 0 {
		target = model.DeviceStatusEnabled
	}
	return s.devices.UpdateStatus(ctx, s.db, deviceID, target, s.clock.Now())
}

// EnrollResult 是 enroll 成功返回给 agent 的结果。
//
// DeviceToken 是长期凭证明文，仅此一次返回；数据库只存其哈希。
type EnrollResult struct {
	DeviceID    string
	DeviceToken string
	Gateway     string
	WSURL       string
	PullURL     string
}

// Enroll 使用设备码完成绑定，整个过程在单事务中完成，保证原子与并发安全。
func (s *Service) Enroll(ctx context.Context, code string, info EnrollInfo) (EnrollResult, error) {
	// 先离线校验签名并解析载荷（不触库）。结构性失败统一为 invalid。
	payload, err := s.codec.Decode(code)
	if err != nil {
		return EnrollResult{}, api.ErrInvalidPairingCode
	}
	codeHash := util.HashTokenHex(payload.PairingToken)

	var result EnrollResult
	txErr := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		now := s.clock.Now()

		pc, err := s.pairings.FindByCodeHash(ctx, tx, codeHash)
		if errors.Is(err, errPairingNotFound) {
			return api.ErrInvalidPairingCode
		}
		if err != nil {
			return err
		}

		// 状态性校验：给出独立 code 便于 agent 提示。
		if pc.IsUsed() {
			return api.ErrUsedPairingCode
		}
		if pc.IsExpired(now) {
			return api.ErrExpiredPairingCode
		}

		d, err := s.devices.GetByID(ctx, tx, pc.DeviceID)
		if errors.Is(err, errDeviceNotFound) {
			return api.ErrInvalidPairingCode
		}
		if err != nil {
			return err
		}
		if d.IsDisabled() {
			return api.ErrDeviceDisabled
		}
		if d.Status == model.DeviceStatusEnabled {
			return api.ErrDeviceAlreadyEnrolled
		}

		// 并发闸门：只有把"未使用"成功翻转为"已使用"的事务才能继续。
		marked, err := s.pairings.MarkUsedIfUnused(ctx, tx, codeHash, now)
		if err != nil {
			return err
		}
		if !marked {
			return api.ErrUsedPairingCode
		}

		// 生成长期凭证：明文仅返回一次，库中只存哈希。
		deviceToken, err := util.RandomToken(s.cfg.DeviceTokenBytes)
		if err != nil {
			return err
		}
		deviceToken = deviceTokenPrefix + deviceToken

		cred := model.DeviceCredential{
			ID:        util.NewID(),
			DeviceID:  d.ID,
			TokenHash: util.HashTokenHex(deviceToken),
			Status:    model.CredentialStatusActive,
			CreatedAt: now,
		}
		if err := s.creds.Insert(ctx, tx, cred); err != nil {
			return err
		}
		if err := s.devices.ApplyEnroll(ctx, tx, d.ID, info, now); err != nil {
			return err
		}

		result = EnrollResult{
			DeviceID:    d.ID,
			DeviceToken: deviceToken,
			Gateway:     s.cfg.Gateway,
			WSURL:       s.cfg.WSURL,
			PullURL:     s.cfg.PullURL,
		}
		return nil
	})
	if txErr != nil {
		return EnrollResult{}, txErr
	}
	return result, nil
}

// pairingSummary 把最近一条设备码归纳为展示用状态。
func pairingSummary(pc model.PairingCode, now time.Time) string {
	switch {
	case pc.IsUsed():
		return PairingSummaryUsed
	case pc.IsExpired(now):
		return PairingSummaryExpired
	default:
		return PairingSummaryActive
	}
}

// mapDeviceErr 把包内的 errDeviceNotFound 映射为对外 API 错误，其余原样透传。
func mapDeviceErr(err error) error {
	if errors.Is(err, errDeviceNotFound) {
		return api.ErrDeviceNotFound
	}
	return err
}

// optionalString 把可空文本字段转换为指针：空串记为"未填写"（nil）。
func optionalString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
