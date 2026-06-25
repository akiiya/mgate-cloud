package hub

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeConn 是 wsConn 的测试替身：不建立真实网络，仅记录关闭状态。
type fakeConn struct {
	closed    atomic.Bool
	closeCode atomic.Int32
}

func (f *fakeConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	<-ctx.Done() // 阻塞直到上下文结束，模拟"无消息"
	return 0, nil, ctx.Err()
}
func (f *fakeConn) Write(ctx context.Context, typ websocket.MessageType, p []byte) error { return nil }
func (f *fakeConn) Close(code websocket.StatusCode, reason string) error {
	f.closed.Store(true)
	f.closeCode.Store(int32(code))
	return nil
}
func (f *fakeConn) SetReadLimit(limit int64) {}

// fakeClock 是可控时钟。
type fakeClock struct{ t atomic.Int64 }

func newFakeClock(t time.Time) *fakeClock    { c := &fakeClock{}; c.t.Store(t.UnixNano()); return c }
func (c *fakeClock) Now() time.Time          { return time.Unix(0, c.t.Load()) }
func (c *fakeClock) advance(d time.Duration) { c.t.Add(int64(d)) }

func TestRegisterReplacesOldConnection(t *testing.T) {
	clk := newFakeClock(time.Now())
	h := New(90*time.Second, clk)

	c1 := NewConnection("dev1", "conn1", "", "", &fakeConn{}, clk.Now())
	c2 := NewConnection("dev1", "conn2", "", "", &fakeConn{}, clk.Now())

	if replaced := h.Register(c1); replaced != nil {
		t.Fatal("首个连接不应有被替换者")
	}
	replaced := h.Register(c2)
	if replaced != c1 {
		t.Fatal("第二个连接应返回被替换的第一个连接")
	}
	if got, _ := h.Get("dev1"); got != c2 {
		t.Error("当前连接应为 c2")
	}
}

func TestUnregisterConnIDGuard(t *testing.T) {
	clk := newFakeClock(time.Now())
	h := New(90*time.Second, clk)
	c1 := NewConnection("dev1", "conn1", "", "", &fakeConn{}, clk.Now())
	c2 := NewConnection("dev1", "conn2", "", "", &fakeConn{}, clk.Now())
	h.Register(c1)
	h.Register(c2) // c2 取代 c1

	// 旧连接 c1 注销时不应误删已接管的 c2。
	h.Unregister("dev1", "conn1")
	if got, ok := h.Get("dev1"); !ok || got != c2 {
		t.Error("旧连接注销不应移除新连接")
	}

	// 正确 connID 注销应移除。
	h.Unregister("dev1", "conn2")
	if _, ok := h.Get("dev1"); ok {
		t.Error("应已移除连接")
	}
}

func TestIsOnlineHeartbeatThreshold(t *testing.T) {
	now := time.Now()
	clk := newFakeClock(now)
	h := New(90*time.Second, clk)

	c := NewConnection("dev1", "conn1", "", "", &fakeConn{}, clk.Now())
	h.Register(c)
	c.MarkHeartbeat(clk.Now())

	if !h.IsOnline("dev1") {
		t.Error("刚心跳应在线")
	}
	clk.advance(91 * time.Second) // 超过 offlineAfter
	if h.IsOnline("dev1") {
		t.Error("超过阈值应离线")
	}
	if h.IsOnline("unknown") {
		t.Error("未知设备应离线")
	}
}

func TestStopClosesAllConnections(t *testing.T) {
	clk := newFakeClock(time.Now())
	h := New(90*time.Second, clk)
	fc := &fakeConn{}
	c := NewConnection("dev1", "conn1", "", "", fc, clk.Now())
	h.Register(c)

	h.Stop()
	if !fc.closed.Load() {
		t.Error("Stop 应关闭所有连接")
	}
}

func TestReapClosesStaleConnections(t *testing.T) {
	now := time.Now()
	clk := newFakeClock(now)
	h := New(90*time.Second, clk)
	fc := &fakeConn{}
	c := NewConnection("dev1", "conn1", "", "", fc, clk.Now())
	h.Register(c)

	clk.advance(91 * time.Second)
	h.reapOnce()
	if !fc.closed.Load() {
		t.Error("超时连接应被 reaper 关闭")
	}
}
