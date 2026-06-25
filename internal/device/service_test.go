package device

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/db"
	"mgate-cloud/internal/model"
	"mgate-cloud/internal/util"
)

// fakeClock 是可控时钟，用于构造"过期"等时间相关场景的确定性测试。
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// testEnv 聚合一次测试所需的服务、时钟、管理员与底层数据库句柄。
type testEnv struct {
	svc     *Service
	clock   *fakeClock
	adminID string
	db      *sql.DB
}

// queryRow 是直查数据库的便捷方法，供测试断言落库结果。
func (e *testEnv) queryRow(t *testing.T, query string, args ...any) *sql.Row {
	t.Helper()
	return e.db.QueryRow(query, args...)
}

func newTestService(t *testing.T) *testEnv {
	t.Helper()
	path := filepath.Join(t.TempDir(), "device_test.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	// 预置一个管理员，满足 device_pairing_codes.created_by_admin_id 外键。
	adminID := util.NewID()
	now := time.Now().UTC()
	if _, err := database.Exec(
		`INSERT INTO admins (id, username, password_hash, status, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		adminID, "admin", "x", model.AdminStatusEnabled, now, now,
	); err != nil {
		t.Fatalf("插入管理员失败: %v", err)
	}

	clock := &fakeClock{t: now}
	svc := NewService(
		database, clock,
		NewDeviceStore(database), NewCredentialStore(database), NewPairingStore(database), NewStatusStore(database),
		NewCodec("test-secret"),
		nil, // presence：单元测试不接入 Hub，在线状态一律 false
		Settings{
			PairingTTL:        30 * time.Minute,
			DeviceTokenBytes:  32,
			PairingTokenBytes: 32,
			Gateway:           "https://cloud.example.com",
			WSURL:             "wss://cloud.example.com/api/agent/ws",
			PullURL:           "https://cloud.example.com/api/agent/pull",
		},
	)
	return &testEnv{svc: svc, clock: clock, adminID: adminID, db: database}
}

var bgCtx = context.Background()

// enrollInfo 是测试用的设备自述信息。
func enrollInfo() EnrollInfo {
	return EnrollInfo{AgentVersion: "0.1.0", MgateVersion: "0.3.7", DeviceModel: "ufi", Hostname: "mgate-001", FirmwareInfo: "debian"}
}

func TestCreateDevice(t *testing.T) {
	env := newTestService(t)
	d, err := env.svc.CreateDevice(bgCtx, "办公室随身 WiFi", "测试设备")
	if err != nil {
		t.Fatalf("创建设备失败: %v", err)
	}
	if d.Status != model.DeviceStatusPending {
		t.Errorf("新设备状态应为 pending，实际 %q", d.Status)
	}
}

func TestCreateDeviceEmptyNameFails(t *testing.T) {
	env := newTestService(t)
	if _, err := env.svc.CreateDevice(bgCtx, "   ", ""); !errors.Is(err, api.ErrBadRequest) {
		t.Errorf("空名称应返回 bad_request，实际 %v", err)
	}
}

func TestGeneratePairingCodeSuccess(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	code, exp, err := env.svc.GeneratePairingCode(bgCtx, d.ID, env.adminID)
	if err != nil {
		t.Fatalf("生成设备码失败: %v", err)
	}
	if code == "" || !exp.After(env.clock.Now()) {
		t.Errorf("设备码或过期时间异常: code=%q exp=%s", code, exp)
	}
}

func TestPairingCodeRejectedForEnabledDevice(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	code, _, _ := env.svc.GeneratePairingCode(bgCtx, d.ID, env.adminID)
	if _, err := env.svc.Enroll(bgCtx, code, enrollInfo()); err != nil {
		t.Fatalf("enroll 失败: %v", err)
	}
	// 已 enabled，应拒绝再次生成。
	if _, _, err := env.svc.GeneratePairingCode(bgCtx, d.ID, env.adminID); !errors.Is(err, api.ErrPairingNotAllowed) {
		t.Errorf("enabled 设备应拒绝生成设备码，实际 %v", err)
	}
}

func TestPairingCodeRejectedForDisabledDevice(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	if err := env.svc.DisableDevice(bgCtx, d.ID); err != nil {
		t.Fatalf("禁用失败: %v", err)
	}
	if _, _, err := env.svc.GeneratePairingCode(bgCtx, d.ID, env.adminID); !errors.Is(err, api.ErrPairingNotAllowed) {
		t.Errorf("disabled 设备应拒绝生成设备码，实际 %v", err)
	}
}

func TestEnrollSuccess(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	code, _, _ := env.svc.GeneratePairingCode(bgCtx, d.ID, env.adminID)

	res, err := env.svc.Enroll(bgCtx, code, enrollInfo())
	if err != nil {
		t.Fatalf("enroll 失败: %v", err)
	}
	if res.DeviceID != d.ID {
		t.Errorf("device_id 不符")
	}
	if res.DeviceToken == "" {
		t.Fatal("应返回 device_token")
	}

	// 设备应变为 enabled，并写入自述信息。
	detail, _ := env.svc.GetDeviceDetail(bgCtx, d.ID)
	if detail.Device.Status != model.DeviceStatusEnabled {
		t.Errorf("enroll 后应为 enabled，实际 %q", detail.Device.Status)
	}
	if detail.Device.LastEnrolledAt == nil {
		t.Error("应记录 last_enrolled_at")
	}
	if detail.ActiveCredentialCount != 1 {
		t.Errorf("应有 1 条有效凭证，实际 %d", detail.ActiveCredentialCount)
	}
}

func TestEnrollStoresOnlyTokenHash(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	code, _, _ := env.svc.GeneratePairingCode(bgCtx, d.ID, env.adminID)
	res, _ := env.svc.Enroll(bgCtx, code, enrollInfo())

	// 直接查库断言：存的是哈希，不是明文。
	var stored string
	if err := env.queryRow(t, `SELECT token_hash FROM device_credentials WHERE device_id = ?`, d.ID).Scan(&stored); err != nil {
		t.Fatalf("查询凭证失败: %v", err)
	}
	if stored == res.DeviceToken {
		t.Fatal("数据库不应保存 device_token 明文")
	}
	if stored != util.HashTokenHex(res.DeviceToken) {
		t.Error("数据库应保存 device_token 的 SHA-256 哈希")
	}
}

func TestEnrollRejectsReuse(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	code, _, _ := env.svc.GeneratePairingCode(bgCtx, d.ID, env.adminID)

	if _, err := env.svc.Enroll(bgCtx, code, enrollInfo()); err != nil {
		t.Fatalf("首次 enroll 失败: %v", err)
	}
	// 同一设备码再次使用：设备已 enabled，应报已绑定。
	if _, err := env.svc.Enroll(bgCtx, code, enrollInfo()); err == nil {
		t.Error("重复使用设备码应失败")
	}
}

func TestEnrollRejectsExpired(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	code, _, _ := env.svc.GeneratePairingCode(bgCtx, d.ID, env.adminID)

	env.clock.advance(31 * time.Minute) // 超过 30 分钟 TTL
	if _, err := env.svc.Enroll(bgCtx, code, enrollInfo()); !errors.Is(err, api.ErrExpiredPairingCode) {
		t.Errorf("过期设备码应返回 expired，实际 %v", err)
	}
}

func TestEnrollRejectsTampered(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	code, _, _ := env.svc.GeneratePairingCode(bgCtx, d.ID, env.adminID)

	tampered := code[:len(code)-1] + "Z"
	if _, err := env.svc.Enroll(bgCtx, tampered, enrollInfo()); !errors.Is(err, api.ErrInvalidPairingCode) {
		t.Errorf("篡改设备码应返回 invalid，实际 %v", err)
	}
}

func TestEnrollRejectsDisabledDevice(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	code, _, _ := env.svc.GeneratePairingCode(bgCtx, d.ID, env.adminID)
	if err := env.svc.DisableDevice(bgCtx, d.ID); err != nil {
		t.Fatalf("禁用失败: %v", err)
	}
	if _, err := env.svc.Enroll(bgCtx, code, enrollInfo()); !errors.Is(err, api.ErrDeviceDisabled) {
		t.Errorf("禁用设备 enroll 应返回 device_disabled，实际 %v", err)
	}
}

func TestDisableThenEnableUnenrolledReturnsPending(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	if err := env.svc.DisableDevice(bgCtx, d.ID); err != nil {
		t.Fatalf("禁用失败: %v", err)
	}
	if err := env.svc.EnableDevice(bgCtx, d.ID); err != nil {
		t.Fatalf("启用失败: %v", err)
	}
	detail, _ := env.svc.GetDeviceDetail(bgCtx, d.ID)
	if detail.Device.Status != model.DeviceStatusPending {
		t.Errorf("未 enroll 设备启用后应为 pending，实际 %q", detail.Device.Status)
	}
}

func TestDisableThenEnableEnrolledReturnsEnabled(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	code, _, _ := env.svc.GeneratePairingCode(bgCtx, d.ID, env.adminID)
	if _, err := env.svc.Enroll(bgCtx, code, enrollInfo()); err != nil {
		t.Fatalf("enroll 失败: %v", err)
	}
	if err := env.svc.DisableDevice(bgCtx, d.ID); err != nil {
		t.Fatalf("禁用失败: %v", err)
	}
	if err := env.svc.EnableDevice(bgCtx, d.ID); err != nil {
		t.Fatalf("启用失败: %v", err)
	}
	detail, _ := env.svc.GetDeviceDetail(bgCtx, d.ID)
	if detail.Device.Status != model.DeviceStatusEnabled {
		t.Errorf("已 enroll 设备启用后应为 enabled，实际 %q", detail.Device.Status)
	}
}

// TestConcurrentEnrollOnlyOneSucceeds 验证同一设备码并发 enroll 只有一个成功。
func TestConcurrentEnrollOnlyOneSucceeds(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	code, _, _ := env.svc.GeneratePairingCode(bgCtx, d.ID, env.adminID)

	const n = 8
	var wg sync.WaitGroup
	var successes atomic.Int32
	tokens := make(chan string, n)

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if res, err := env.svc.Enroll(bgCtx, code, enrollInfo()); err == nil {
				successes.Add(1)
				tokens <- res.DeviceToken
			}
		}()
	}
	wg.Wait()
	close(tokens)

	if got := successes.Load(); got != 1 {
		t.Fatalf("并发 enroll 应只成功一次，实际 %d", got)
	}
	// 仅应签发一个 device_token。
	if len(tokens) != 1 {
		t.Errorf("应只签发一个 device_token，实际 %d", len(tokens))
	}
	// 数据库中该设备应只有一条凭证。
	var credCount int
	if err := env.queryRow(t, `SELECT COUNT(*) FROM device_credentials WHERE device_id = ?`, d.ID).Scan(&credCount); err != nil {
		t.Fatalf("查询凭证数失败: %v", err)
	}
	if credCount != 1 {
		t.Errorf("应只有 1 条凭证，实际 %d", credCount)
	}
}

func TestDisableIsIdempotent(t *testing.T) {
	env := newTestService(t)
	d, _ := env.svc.CreateDevice(bgCtx, "dev", "")
	if err := env.svc.DisableDevice(bgCtx, d.ID); err != nil {
		t.Fatalf("首次禁用失败: %v", err)
	}
	if err := env.svc.DisableDevice(bgCtx, d.ID); err != nil {
		t.Errorf("重复禁用应幂等成功，实际 %v", err)
	}
}
