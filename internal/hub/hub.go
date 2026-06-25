package hub

import (
	"sync"
	"time"

	"github.com/coder/websocket"

	"mgate-cloud/internal/util"
)

// Hub 是设备 WebSocket 连接的进程内注册表。
//
// 约定：online（在线）是进程内的瞬时状态，不持久化。Hub 重启后所有设备自然离线，
// 但数据库中的 last_seen_at 等时间点保留。
type Hub struct {
	mu           sync.RWMutex
	conns        map[string]*Connection // device_id -> 当前活跃连接
	offlineAfter time.Duration
	clock        util.Clock

	stopOnce sync.Once
	stopped  chan struct{}
}

// New 构造 Hub。offlineAfter 为离线判定阈值。
func New(offlineAfter time.Duration, clock util.Clock) *Hub {
	return &Hub{
		conns:        make(map[string]*Connection),
		offlineAfter: offlineAfter,
		clock:        clock,
		stopped:      make(chan struct{}),
	}
}

// Register 登记一条新连接；若该设备已有连接，返回旧连接（由调用方负责关闭）。
//
// 同一设备只保留一个活跃连接：新连接接入即取代旧连接。把"关闭旧连接"留给调用方
// 在锁外执行，避免在持有 Hub 锁时做可能阻塞的网络关闭操作。
func (h *Hub) Register(c *Connection) (replaced *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	old := h.conns[c.DeviceID]
	h.conns[c.DeviceID] = c
	return old
}

// Unregister 注销连接，但仅当当前登记的正是该 connID 时才删除。
//
// 这一守卫至关重要：被取代的旧连接在其读循环结束时也会调用 Unregister，
// 若不校验 connID，旧连接的注销会误删刚接管的新连接。
func (h *Hub) Unregister(deviceID, connID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cur, ok := h.conns[deviceID]; ok && cur.ConnID == connID {
		delete(h.conns, deviceID)
	}
}

// Get 返回设备当前连接。
func (h *Hub) Get(deviceID string) (*Connection, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.conns[deviceID]
	return c, ok
}

// IsOnline 判断设备是否在线：存在活跃连接，且最近心跳在阈值内。
func (h *Hub) IsOnline(deviceID string) bool {
	h.mu.RLock()
	c, ok := h.conns[deviceID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	return h.clock.Now().Sub(c.LastHeartbeat()) <= h.offlineAfter
}

// OnlineCount 返回当前在线设备数（供 Dashboard 统计）。
func (h *Hub) OnlineCount() int {
	now := h.clock.Now()
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for _, c := range h.conns {
		if now.Sub(c.LastHeartbeat()) <= h.offlineAfter {
			n++
		}
	}
	return n
}

// snapshotConns 在锁内复制当前连接切片，供锁外遍历（关闭/清理）使用。
func (h *Hub) snapshotConns() []*Connection {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Connection, 0, len(h.conns))
	for _, c := range h.conns {
		out = append(out, c)
	}
	return out
}

// Stop 停止后台清理并关闭所有连接（用于服务优雅关闭）。幂等。
func (h *Hub) Stop() {
	h.stopOnce.Do(func() {
		close(h.stopped)
		for _, c := range h.snapshotConns() {
			c.Close(websocket.StatusGoingAway, "server shutting down")
		}
	})
}
