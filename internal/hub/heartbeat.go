package hub

import (
	"log"
	"time"

	"github.com/coder/websocket"
)

// StartReaper 启动后台清理：周期性扫描并关闭"心跳超时"的连接。
//
// 读循环本身也会设置读超时作为主要的失活检测；本 reaper 是兜底，确保即便底层
// 读阻塞未及时返回，超时连接也会被主动关闭并释放，从而让在线状态尽快归于离线。
// 调用方在服务关闭时通过 Hub.Stop() 终止本 goroutine。
func (h *Hub) StartReaper(interval time.Duration) {
	// 防御：非正间隔会让 time.NewTicker panic，回退到一个安全默认值。
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-h.stopped:
				return
			case <-ticker.C:
				h.reapOnce()
			}
		}
	}()
}

// reapOnce 关闭所有心跳已超过 offlineAfter 的连接。
func (h *Hub) reapOnce() {
	now := h.clock.Now()
	for _, c := range h.snapshotConns() {
		if now.Sub(c.LastHeartbeat()) > h.offlineAfter {
			log.Printf("hub: 清理超时连接 device_id=%s conn_id=%s", c.DeviceID, c.ConnID)
			c.Close(websocket.StatusGoingAway, "heartbeat timeout")
			// 注意：不在此删除映射；连接关闭后其读循环会自行 Unregister（带 connID 守卫）。
		}
	}
}
