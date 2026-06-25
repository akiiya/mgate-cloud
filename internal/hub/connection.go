// Package hub 维护进程内的 Agent WebSocket 连接注册表与在线状态。
//
// 设计原则：hub 只关心"连接的注册、替换、超时清理与在线判断"，
// 不接触数据库、不理解业务消息——这些由 agent 层处理。这样 hub 保持纯粹、易测。
package hub

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// wsConn 抽象底层 WebSocket 连接所需的方法子集。
//
// 抽象成接口的目的：让 Connection 的注册/替换/关闭逻辑可在不建立真实网络连接的
// 情况下进行单元测试（注入 fake）。*websocket.Conn 天然满足该接口。
type wsConn interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
	Close(code websocket.StatusCode, reason string) error
	SetReadLimit(limit int64)
}

// Connection 表示一条已认证的设备 WebSocket 连接及其元数据。
type Connection struct {
	DeviceID    string
	ConnID      string
	RemoteAddr  string
	UserAgent   string
	ConnectedAt time.Time

	ws wsConn
	// writeMu 串行化写操作：coder/websocket 不允许并发写。
	writeMu sync.Mutex
	// lastHeartbeat 以 UnixNano 原子存储，读写无需加锁，避免阻塞 Hub。
	lastHeartbeat atomic.Int64
	// closeOnce 保证关闭幂等：重复关闭只生效一次。
	closeOnce sync.Once
}

// NewConnection 构造连接对象并初始化心跳时间为建连时刻。
func NewConnection(deviceID, connID, remoteAddr, userAgent string, ws wsConn, now time.Time) *Connection {
	c := &Connection{
		DeviceID:    deviceID,
		ConnID:      connID,
		RemoteAddr:  remoteAddr,
		UserAgent:   userAgent,
		ConnectedAt: now,
		ws:          ws,
	}
	c.lastHeartbeat.Store(now.UnixNano())
	return c
}

// Read 读取一条消息（原始字节）。调用方应自行控制 ctx 超时以实现读超时。
func (c *Connection) Read(ctx context.Context) ([]byte, error) {
	_, data, err := c.ws.Read(ctx)
	return data, err
}

// WriteJSON 序列化并发送一条文本消息，写操作串行化。
func (c *Connection) WriteJSON(ctx context.Context, raw []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.ws.Write(ctx, websocket.MessageText, raw)
}

// MarkHeartbeat 更新最近心跳时间（原子）。
func (c *Connection) MarkHeartbeat(t time.Time) {
	c.lastHeartbeat.Store(t.UnixNano())
}

// LastHeartbeat 返回最近心跳时间。
func (c *Connection) LastHeartbeat() time.Time {
	return time.Unix(0, c.lastHeartbeat.Load())
}

// Close 关闭底层连接，幂等。reason 会作为关闭原因发送给对端。
func (c *Connection) Close(code websocket.StatusCode, reason string) {
	c.closeOnce.Do(func() {
		_ = c.ws.Close(code, reason)
	})
}
