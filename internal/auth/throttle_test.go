package auth_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"mgate-cloud/internal/auth"
	"mgate-cloud/internal/db"
)

// fakeClock 是可手动推进的时钟，用于确定性地测试窗口与封禁时长。
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }
func (c *fakeClock) advance(d time.Duration) {
	c.t = c.t.Add(d)
}

func newThrottle(t *testing.T, clock *fakeClock, s auth.ThrottleSettings) *auth.LoginThrottle {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "throttle.db"))
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return auth.NewLoginThrottle(auth.NewLoginThrottleStore(database, clock), clock, s)
}

func defaultSettings() auth.ThrottleSettings {
	return auth.ThrottleSettings{
		Enabled:       true,
		MaxFailures:   3,
		FailureWindow: 15 * time.Minute,
		BaseBan:       1 * time.Hour,
		MaxBan:        24 * time.Hour,
		OffenseReset:  24 * time.Hour,
	}
}

func TestThrottleBansAfterThreshold(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0).UTC()}
	tr := newThrottle(t, clock, defaultSettings())
	ctx := context.Background()
	ip := "1.2.3.4"

	if allowed, _ := tr.Allow(ctx, ip); !allowed {
		t.Fatal("初始应允许登录")
	}
	// 前两次失败不应封禁。
	for i := 0; i < 2; i++ {
		if banned, _ := tr.RecordFailure(ctx, ip); banned {
			t.Fatalf("第 %d 次失败不应触发封禁", i+1)
		}
	}
	if allowed, _ := tr.Allow(ctx, ip); !allowed {
		t.Fatal("未达阈值前应仍允许")
	}
	// 第三次失败达到阈值，触发首次封禁 = BaseBan。
	banned, banFor := tr.RecordFailure(ctx, ip)
	if !banned {
		t.Fatal("达到阈值应触发封禁")
	}
	if banFor != time.Hour {
		t.Fatalf("首次封禁应为 1h，实际 %s", banFor)
	}
	allowed, retry := tr.Allow(ctx, ip)
	if allowed {
		t.Fatal("封禁期间应拒绝登录")
	}
	if retry <= 0 || retry > time.Hour {
		t.Fatalf("剩余封禁时长异常: %s", retry)
	}
}

func TestThrottleEscalatesDuration(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0).UTC()}
	tr := newThrottle(t, clock, defaultSettings())
	ctx := context.Background()
	ip := "9.9.9.9"

	ban := func() time.Duration {
		var d time.Duration
		for i := 0; i < 3; i++ {
			_, d = tr.RecordFailure(ctx, ip)
		}
		return d
	}

	if d := ban(); d != time.Hour {
		t.Fatalf("第一次封禁应 1h，实际 %s", d)
	}
	clock.advance(time.Hour + time.Minute) // 解封后继续
	if d := ban(); d != 2*time.Hour {
		t.Fatalf("第二次封禁应升级为 2h，实际 %s", d)
	}
	clock.advance(2*time.Hour + time.Minute)
	if d := ban(); d != 4*time.Hour {
		t.Fatalf("第三次封禁应升级为 4h，实际 %s", d)
	}
}

func TestThrottleSuccessResets(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0).UTC()}
	tr := newThrottle(t, clock, defaultSettings())
	ctx := context.Background()
	ip := "5.5.5.5"

	tr.RecordFailure(ctx, ip)
	tr.RecordFailure(ctx, ip)
	tr.RecordSuccess(ctx, ip) // 清零
	// 成功后再两次失败：计数从 0 重新开始，不应封禁。
	tr.RecordFailure(ctx, ip)
	if banned, _ := tr.RecordFailure(ctx, ip); banned {
		t.Fatal("成功登录应已重置失败计数，不应被封禁")
	}
}

func TestThrottleWindowResetsConsecutiveCount(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0).UTC()}
	tr := newThrottle(t, clock, defaultSettings())
	ctx := context.Background()
	ip := "7.7.7.7"

	tr.RecordFailure(ctx, ip)
	tr.RecordFailure(ctx, ip)
	clock.advance(16 * time.Minute) // 超过 15min 窗口
	// 窗口过期后计数重置，这两次不应累加到上面两次而触发封禁。
	tr.RecordFailure(ctx, ip)
	if banned, _ := tr.RecordFailure(ctx, ip); banned {
		t.Fatal("窗口过期应重置连续计数，不应封禁")
	}
}

func TestThrottleDisabled(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0).UTC()}
	s := defaultSettings()
	s.Enabled = false
	tr := newThrottle(t, clock, s)
	ctx := context.Background()
	ip := "8.8.8.8"

	for i := 0; i < 10; i++ {
		if banned, _ := tr.RecordFailure(ctx, ip); banned {
			t.Fatal("禁用限流时不应封禁")
		}
	}
	if allowed, _ := tr.Allow(ctx, ip); !allowed {
		t.Fatal("禁用限流时应始终允许")
	}
}
