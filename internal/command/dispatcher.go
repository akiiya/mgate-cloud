package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"mgate-cloud/internal/model"
	"mgate-cloud/internal/util"
)

// Dispatcher 负责"先落库、再投递"：把已落库的命令通过 Deliverer 推送给在线设备（WS 主通道）。
//
// 关键不变量：命令一定先在 commands 表存在，才会被投递；投递失败会释放租约（带退避），
// 命令保持 pending、绝不丢失，等待 Pull 或 WS 重连重试。
type Dispatcher struct {
	db           *sql.DB
	store        *Store
	deliverer    Deliverer
	clock        util.Clock
	instanceID   string
	leaseDur     time.Duration
	retryBackoff time.Duration
}

// NewDispatcher 构造投递器。
func NewDispatcher(db *sql.DB, store *Store, deliverer Deliverer, clock util.Clock, leaseDur, retryBackoff time.Duration) *Dispatcher {
	if leaseDur <= 0 {
		leaseDur = 60 * time.Second
	}
	return &Dispatcher{
		db: db, store: store, deliverer: deliverer, clock: clock,
		instanceID: util.NewID(), leaseDur: leaseDur, retryBackoff: retryBackoff,
	}
}

// TryDeliver 尝试经 WS 投递一条命令，返回是否成功投递（状态已转为 sent）。
//
// 流程：抢占租约（pending→leased，attempts++）→ 经 WS 发送 → 成功则 sent，
// 失败则释放租约回 pending（带退避），等待后续重试。
func (d *Dispatcher) TryDeliver(ctx context.Context, cmd model.Command) (bool, error) {
	now := d.clock.Now()

	leased, err := d.store.Lease(ctx, d.db, cmd.ID, "ws:"+d.instanceID, now.Add(d.leaseDur), now)
	if err != nil {
		return false, err
	}
	if !leased {
		// 命令不可领取（非 pending、退避未到、或已被其他通道领取）。
		return false, nil
	}

	payload := DeliverPayload{
		CommandID:  cmd.ID,
		Action:     cmd.Action,
		Params:     json.RawMessage(cmd.ParamsJSON),
		TimeoutSec: cmd.TimeoutSec,
	}
	if err := d.deliverer.Deliver(cmd.DeviceID, payload); err != nil {
		// 投递失败：释放租约并设置退避，命令回到 pending、不丢失。
		_ = d.store.ReleaseLease(ctx, d.db, cmd.ID, d.clock.Now().Add(d.retryBackoff), d.clock.Now())
		return false, err
	}

	return d.store.MarkSent(ctx, d.db, cmd.ID, d.clock.Now())
}
