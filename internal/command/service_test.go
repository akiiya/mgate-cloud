package command

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/db"
	"mgate-cloud/internal/model"
	"mgate-cloud/internal/util"
)

var bg = context.Background()

// fakeClock 可控时钟。
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// fakeDeliverer 模拟投递：online=false 时返回 ErrNotOnline。
type fakeDeliverer struct {
	online    bool
	delivered []DeliverPayload
}

func (d *fakeDeliverer) Deliver(deviceID string, p DeliverPayload) error {
	if !d.online {
		return ErrNotOnline
	}
	d.delivered = append(d.delivered, p)
	return nil
}

// fakeGate 模拟设备就绪检查与在线判断。
type fakeGate struct {
	dev    model.Device
	err    error
	online bool
}

func (g *fakeGate) EnsureCommandable(ctx context.Context, deviceID string) (model.Device, error) {
	if g.err != nil {
		return model.Device{}, g.err
	}
	return g.dev, nil
}

func (g *fakeGate) IsOnline(deviceID string) bool { return g.online }

type cmdEnv struct {
	svc       *Service
	db        *sql.DB
	clock     *fakeClock
	deliverer *fakeDeliverer
	gate      *fakeGate
	deviceID  string
	adminID   string
}

func newCmdEnv(t *testing.T) *cmdEnv {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cmd_test.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	now := time.Now().UTC()
	adminID := util.NewID()
	deviceID := util.NewID()
	if _, err := database.Exec(`INSERT INTO admins (id,username,password_hash,status,created_at,updated_at) VALUES (?,?,?,?,?,?)`,
		adminID, "admin", "x", model.AdminStatusEnabled, now, now); err != nil {
		t.Fatalf("插入管理员失败: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO devices (id,name,status,created_at,updated_at) VALUES (?,?,?,?,?)`,
		deviceID, "dev", model.DeviceStatusEnabled, now, now); err != nil {
		t.Fatalf("插入设备失败: %v", err)
	}

	clock := &fakeClock{t: now}
	store := NewStore(database)
	results := NewResultStore(database)
	deliverer := &fakeDeliverer{online: true}
	gate := &fakeGate{dev: model.Device{ID: deviceID, Status: model.DeviceStatusEnabled}, online: true}
	dispatcher := NewDispatcher(database, store, deliverer, clock, 60*time.Second, 10*time.Second)
	svc := NewService(database, clock, store, results, dispatcher, gate, audit.NewService(database, clock), Settings{
		DefaultTimeout:    60 * time.Second,
		MaxTimeout:        300 * time.Second,
		ResultMaxBytes:    262144,
		PendingTTL:        10 * time.Minute,
		DefaultMaxAttempt: 3,
		LeaseDuration:     60 * time.Second,
		RetryBackoff:      10 * time.Second,
		PullMaxCommands:   10,
	})
	return &cmdEnv{svc: svc, db: database, clock: clock, deliverer: deliverer, gate: gate, deviceID: deviceID, adminID: adminID}
}

func (e *cmdEnv) create(t *testing.T, action, params string) model.Command {
	t.Helper()
	cmd, _, err := e.svc.CreateCommand(bg, CreateInput{
		DeviceID: e.deviceID, AdminID: e.adminID, Action: action, RawParams: []byte(params),
	})
	if err != nil {
		t.Fatalf("创建命令失败: %v", err)
	}
	return cmd
}

func (e *cmdEnv) status(t *testing.T, id string) string {
	t.Helper()
	var s string
	if err := e.db.QueryRow(`SELECT status FROM commands WHERE id=?`, id).Scan(&s); err != nil {
		t.Fatalf("查询状态失败: %v", err)
	}
	return s
}

func TestCreateCommandOnlineSucceeds(t *testing.T) {
	e := newCmdEnv(t)
	cmd := e.create(t, "ap.status", "{}")
	if cmd.Status != model.CommandStatusSent {
		t.Errorf("在线创建应投递为 sent，实际 %q", cmd.Status)
	}
	if len(e.deliverer.delivered) != 1 {
		t.Errorf("应投递一次，实际 %d", len(e.deliverer.delivered))
	}
}

func TestCreateCommandOfflineQueued(t *testing.T) {
	e := newCmdEnv(t)
	e.gate.online = false // 离线
	cmd, hint, err := e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "ap.status", RawParams: []byte("{}")})
	if err != nil {
		t.Fatalf("离线设备创建命令不应失败: %v", err)
	}
	if cmd.Status != model.CommandStatusPending {
		t.Errorf("离线命令应为 pending，实际 %q", cmd.Status)
	}
	if hint != HintOfflineWaitForPull {
		t.Errorf("离线 hint 应为 %q，实际 %q", HintOfflineWaitForPull, hint)
	}
}

func TestCreateCommandNotReadyFails(t *testing.T) {
	e := newCmdEnv(t)
	e.gate.err = api.ErrDeviceNotReady
	if _, _, err := e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "ap.status", RawParams: []byte("{}")}); !errors.Is(err, api.ErrDeviceNotReady) {
		t.Errorf("应返回 device_not_ready，实际 %v", err)
	}
}

func TestCreateCommandInvalidAction(t *testing.T) {
	e := newCmdEnv(t)
	if _, _, err := e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "exec.raw", RawParams: []byte("{}")}); !errors.Is(err, api.ErrInvalidAction) {
		t.Errorf("非白名单应返回 invalid_action，实际 %v", err)
	}
}

func TestCreateCommandInvalidParams(t *testing.T) {
	e := newCmdEnv(t)
	if _, _, err := e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "ap.status", RawParams: []byte(`{"shell":"x"}`)}); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("非法参数应返回 invalid_params，实际 %v", err)
	}
}

func TestCreateCommandTimeoutTooLarge(t *testing.T) {
	e := newCmdEnv(t)
	if _, _, err := e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "ap.status", RawParams: []byte("{}"), TimeoutSec: 9999}); !errors.Is(err, api.ErrTimeoutTooLarge) {
		t.Errorf("超时过大应返回 timeout_too_large，实际 %v", err)
	}
}

func TestCreateCommandDeliverFailKeepsPending(t *testing.T) {
	e := newCmdEnv(t)
	e.deliverer.online = false // 在线但 WS 投递失败
	cmd, hint, err := e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "ap.status", RawParams: []byte("{}")})
	if err != nil {
		t.Fatalf("创建不应失败: %v", err)
	}
	if cmd.Status != model.CommandStatusPending {
		t.Errorf("投递失败命令应保持 pending，实际 %q", cmd.Status)
	}
	if hint != HintQueuedForRetry {
		t.Errorf("投递失败 hint 应为 %q，实际 %q", HintQueuedForRetry, hint)
	}
}

func TestAckAcceptedThenStatusAcked(t *testing.T) {
	e := newCmdEnv(t)
	cmd := e.create(t, "ap.status", "{}")
	e.svc.HandleAck(bg, e.deviceID, AckInput{CommandID: cmd.ID, Accepted: true}, "", "", "")
	if e.status(t, cmd.ID) != model.CommandStatusAcked {
		t.Errorf("ack accepted 后应为 acked，实际 %q", e.status(t, cmd.ID))
	}
}

func TestAckRejectedThenFailedWithResult(t *testing.T) {
	e := newCmdEnv(t)
	cmd := e.create(t, "ap.status", "{}")
	e.svc.HandleAck(bg, e.deviceID, AckInput{CommandID: cmd.ID, Accepted: false, Message: "unsupported"}, "", "", "")
	if e.status(t, cmd.ID) != model.CommandStatusFailed {
		t.Errorf("ack rejected 后应为 failed，实际 %q", e.status(t, cmd.ID))
	}
	var n int
	e.db.QueryRow(`SELECT COUNT(*) FROM command_results WHERE command_id=?`, cmd.ID).Scan(&n)
	if n != 1 {
		t.Errorf("rejected 应保存一条结果，实际 %d", n)
	}
}

func TestResultSucceeded(t *testing.T) {
	e := newCmdEnv(t)
	cmd := e.create(t, "ap.status", "{}")
	e.svc.HandleResult(bg, e.deviceID, ResultInput{CommandID: cmd.ID, Status: "succeeded", Stdout: "ok", Result: []byte(`{"state":"running"}`)}, "", "", "")
	if e.status(t, cmd.ID) != model.CommandStatusSucceeded {
		t.Errorf("result succeeded 后应为 succeeded，实际 %q", e.status(t, cmd.ID))
	}
}

func TestResultFailed(t *testing.T) {
	e := newCmdEnv(t)
	cmd := e.create(t, "ap.status", "{}")
	e.svc.HandleResult(bg, e.deviceID, ResultInput{CommandID: cmd.ID, Status: "failed", Stderr: "boom"}, "", "", "")
	if e.status(t, cmd.ID) != model.CommandStatusFailed {
		t.Errorf("result failed 后应为 failed，实际 %q", e.status(t, cmd.ID))
	}
}

func TestResultWrongDeviceIgnored(t *testing.T) {
	e := newCmdEnv(t)
	cmd := e.create(t, "ap.status", "{}")
	e.svc.HandleResult(bg, "other-device", ResultInput{CommandID: cmd.ID, Status: "succeeded"}, "", "", "")
	if e.status(t, cmd.ID) == model.CommandStatusSucceeded {
		t.Error("其他设备的 result 不应改变命令状态")
	}
}

func TestResultUnknownCommandNoPanic(t *testing.T) {
	e := newCmdEnv(t)
	// 不应 panic。
	e.svc.HandleResult(bg, e.deviceID, ResultInput{CommandID: "cmd_nope", Status: "succeeded"}, "", "", "")
}

func TestResultIdempotent(t *testing.T) {
	e := newCmdEnv(t)
	cmd := e.create(t, "ap.status", "{}")
	e.svc.HandleResult(bg, e.deviceID, ResultInput{CommandID: cmd.ID, Status: "succeeded", Stdout: "first"}, "", "", "")
	e.svc.HandleResult(bg, e.deviceID, ResultInput{CommandID: cmd.ID, Status: "failed", Stdout: "second"}, "", "", "")
	var n int
	e.db.QueryRow(`SELECT COUNT(*) FROM command_results WHERE command_id=?`, cmd.ID).Scan(&n)
	if n != 1 {
		t.Errorf("重复 result 应只保留一条，实际 %d", n)
	}
	if e.status(t, cmd.ID) != model.CommandStatusSucceeded {
		t.Errorf("首个 result 应胜出，状态应仍为 succeeded，实际 %q", e.status(t, cmd.ID))
	}
}

func TestResultTruncation(t *testing.T) {
	e := newCmdEnv(t)
	// 调小上限以触发截断。
	e.svc.cfg.ResultMaxBytes = 10
	cmd := e.create(t, "ap.status", "{}")
	long := "0123456789ABCDEFGHIJ" // 20 字节
	e.svc.HandleResult(bg, e.deviceID, ResultInput{CommandID: cmd.ID, Status: "succeeded", Stdout: long}, "", "", "")

	var stdout string
	var truncated int
	e.db.QueryRow(`SELECT COALESCE(stdout,''), truncated FROM command_results WHERE command_id=?`, cmd.ID).Scan(&stdout, &truncated)
	if len(stdout) > 10 {
		t.Errorf("stdout 应被截断到 <=10，实际 %d", len(stdout))
	}
	if truncated != 1 {
		t.Error("应标记 truncated=1")
	}
}

func TestCancelPendingSucceeds(t *testing.T) {
	e := newCmdEnv(t)
	e.gate.online = false // 离线 → 保持 pending
	cmd, _, _ := e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "ap.status", RawParams: []byte("{}")})
	if err := e.svc.CancelCommand(bg, cmd.ID, e.adminID, "", "", ""); err != nil {
		t.Fatalf("取消 pending 应成功: %v", err)
	}
	if e.status(t, cmd.ID) != model.CommandStatusCanceled {
		t.Errorf("应为 canceled，实际 %q", e.status(t, cmd.ID))
	}
}

func TestCancelTerminalFails(t *testing.T) {
	e := newCmdEnv(t)
	cmd := e.create(t, "ap.status", "{}")
	e.svc.HandleResult(bg, e.deviceID, ResultInput{CommandID: cmd.ID, Status: "succeeded"}, "", "", "")
	if err := e.svc.CancelCommand(bg, cmd.ID, e.adminID, "", "", ""); !errors.Is(err, api.ErrCommandNotCancelable) {
		t.Errorf("取消终态命令应失败，实际 %v", err)
	}
}

func TestReaperTimeoutWhenAttemptsExhausted(t *testing.T) {
	e := newCmdEnv(t)
	e.svc.cfg.DefaultMaxAttempt = 1 // 仅一次尝试 → 超时后直接 timeout，不重试
	cmd, _, err := e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "ap.status", RawParams: []byte("{}"), TimeoutSec: 1})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if cmd.Status != model.CommandStatusSent {
		t.Fatalf("应为 sent，实际 %q", cmd.Status)
	}
	e.clock.advance(2 * time.Second) // 超过 1s 超时
	e.svc.ReapOnce(bg)
	if e.status(t, cmd.ID) != model.CommandStatusTimeout {
		t.Errorf("attempts 耗尽后应为 timeout，实际 %q", e.status(t, cmd.ID))
	}
}

func TestReaperRetriesWhenAttemptsLeft(t *testing.T) {
	e := newCmdEnv(t)
	// 默认 max_attempts=3；create 后 attempts=1，超时后应重试回 pending。
	cmd, _, err := e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "ap.status", RawParams: []byte("{}"), TimeoutSec: 1})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	e.clock.advance(2 * time.Second)
	e.svc.ReapOnce(bg)
	if e.status(t, cmd.ID) != model.CommandStatusPending {
		t.Errorf("attempts 未耗尽应重试回 pending，实际 %q", e.status(t, cmd.ID))
	}
	// 应写 command.retry 审计。
	var n int
	e.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='command.retry' AND target_id=?`, cmd.ID).Scan(&n)
	if n == 0 {
		t.Error("应写 command.retry 审计")
	}
}

func TestReaperLeaseExpiredRetries(t *testing.T) {
	e := newCmdEnv(t)
	e.deliverer.online = false // 投递失败 → 命令进入 pending（带退避 lease_until）
	cmd, _, _ := e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "ap.status", RawParams: []byte("{}")})
	// 直接构造 leased 且租约过期的场景：手动置为 leased。
	past := e.clock.Now().Add(-time.Minute)
	if _, err := e.db.Exec(`UPDATE commands SET status='leased', lease_until=?, attempts=1 WHERE id=?`, past, cmd.ID); err != nil {
		t.Fatalf("构造 leased 失败: %v", err)
	}
	e.svc.ReapOnce(bg)
	if e.status(t, cmd.ID) != model.CommandStatusPending {
		t.Errorf("租约过期且可重试应回 pending，实际 %q", e.status(t, cmd.ID))
	}
}

func TestReaperMarksExpired(t *testing.T) {
	e := newCmdEnv(t)
	e.gate.online = false // 离线 → 保持 pending
	e.svc.cfg.PendingTTL = 1 * time.Second
	cmd, _, _ := e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "ap.status", RawParams: []byte("{}")})
	e.clock.advance(2 * time.Second)
	e.svc.ReapOnce(bg)
	if e.status(t, cmd.ID) != model.CommandStatusExpired {
		t.Errorf("pending 超 TTL 应为 expired，实际 %q", e.status(t, cmd.ID))
	}
}

func TestLeaseForPullDeliversPending(t *testing.T) {
	e := newCmdEnv(t)
	e.gate.online = false // 离线创建 pending
	cmd, _, _ := e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "ap.status", RawParams: []byte("{}")})

	payloads, err := e.svc.LeaseForPull(bg, e.deviceID, "req_1", 3)
	if err != nil {
		t.Fatalf("LeaseForPull 失败: %v", err)
	}
	if len(payloads) != 1 || payloads[0].CommandID != cmd.ID {
		t.Fatalf("应领取到 1 条命令，实际 %d", len(payloads))
	}
	if e.status(t, cmd.ID) != model.CommandStatusSent {
		t.Errorf("领取后应为 sent，实际 %q", e.status(t, cmd.ID))
	}
	// 第二次领取不应再拿到同一命令（已 sent）。
	again, _ := e.svc.LeaseForPull(bg, e.deviceID, "req_2", 3)
	if len(again) != 0 {
		t.Errorf("已领取命令不应被重复领取，实际 %d", len(again))
	}
}

func TestLeaseForPullOnlyOwnDevice(t *testing.T) {
	e := newCmdEnv(t)
	e.gate.online = false
	e.svc.CreateCommand(bg, CreateInput{DeviceID: e.deviceID, AdminID: e.adminID, Action: "ap.status", RawParams: []byte("{}")})
	// 其他设备不应领取到本设备的命令。
	payloads, _ := e.svc.LeaseForPull(bg, "other-device", "req_x", 3)
	if len(payloads) != 0 {
		t.Errorf("不应领取其他设备的命令，实际 %d", len(payloads))
	}
}
