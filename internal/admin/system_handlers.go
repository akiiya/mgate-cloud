package admin

import (
	"net/http"

	"mgate-cloud/internal/api"
)

// SystemInfo 是返回给前端「设置 / 系统」页的只读系统信息。
//
// 刻意只暴露非敏感、用于产品化展示的字段：版本、运行模式、更新通道等。
// 不包含任何 secret、口令、token、数据库内容。
type SystemInfo struct {
	Version       string `json:"version"`
	Mode          string `json:"mode"`
	UpdateChannel string `json:"update_channel"`
	UpdateEnabled bool   `json:"update_enabled"`
	CookieSecure  bool   `json:"cookie_secure"`
}

// SystemHandlers 提供系统信息只读接口。
type SystemHandlers struct {
	info SystemInfo
}

// NewSystemHandlers 用启动时的快照构造系统信息处理器。
//
// 取启动快照即可：这些值在进程生命周期内不变，无需每次请求重新读取配置。
func NewSystemHandlers(info SystemInfo) *SystemHandlers {
	return &SystemHandlers{info: info}
}

// Info 返回只读系统信息（需登录）。
func (h *SystemHandlers) Info(w http.ResponseWriter, r *http.Request) {
	api.WriteSuccess(w, http.StatusOK, h.info)
}
