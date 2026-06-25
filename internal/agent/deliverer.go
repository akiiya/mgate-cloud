package agent

import (
	"context"
	"time"

	"mgate-cloud/internal/command"
	"mgate-cloud/internal/hub"
)

// CommandDeliverer 通过 Hub 把命令以 command.deliver 信封投递给设备的在线连接。
//
// 它实现 command.Deliverer 接口。把实现放在 agent 包（而非 command 包），是为了
// 让 command 不依赖 agent/hub 的具体类型，避免导入环。
type CommandDeliverer struct {
	hub *hub.Hub
}

// NewCommandDeliverer 构造投递器。
func NewCommandDeliverer(h *hub.Hub) *CommandDeliverer {
	return &CommandDeliverer{hub: h}
}

// deliverWriteTimeout 限制单次投递写入时长，避免慢连接拖住投递流程。
const deliverWriteTimeout = 5 * time.Second

// Deliver 把命令投递给设备的在线连接；设备不在线返回 command.ErrNotOnline。
func (d *CommandDeliverer) Deliver(deviceID string, payload command.DeliverPayload) error {
	conn, ok := d.hub.Get(deviceID)
	if !ok {
		return command.ErrNotOnline
	}

	raw, err := newEnvelope(time.Now().UTC(), typeCommandDeliver, deviceID, payload)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), deliverWriteTimeout)
	defer cancel()
	return conn.WriteJSON(ctx, raw)
}
