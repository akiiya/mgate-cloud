package admin

import (
	"net/http"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/model"
	"mgate-cloud/internal/update"
)

// UpdateHandlers 处理管理员侧的检查更新 / 自更新。
type UpdateHandlers struct {
	svc   *update.Service
	audit *audit.Service
}

// NewUpdateHandlers 构造更新处理器。
func NewUpdateHandlers(svc *update.Service, auditService *audit.Service) *UpdateHandlers {
	return &UpdateHandlers{svc: svc, audit: auditService}
}

// Check 查询是否有可用更新。
func (h *UpdateHandlers) Check(w http.ResponseWriter, r *http.Request) {
	if !h.svc.Enabled() {
		api.WriteError(w, api.ErrUpdateDisabled)
		return
	}
	res, err := h.svc.Check(r.Context())
	if err != nil {
		api.WriteError(w, api.NewError("update_check_failed", err.Error(), http.StatusBadGateway))
		return
	}
	admin, _ := AdminFrom(r.Context())
	h.record(r, admin.ID, model.ActionUpdateChecked, map[string]any{
		"current": res.CurrentVersion, "latest": res.LatestVersion, "has_update": res.HasUpdate,
	})
	api.WriteSuccess(w, http.StatusOK, res)
}

// Apply 下载并安装更新（下载 → 校验 SHA256 → 解压 → 备份 → 替换二进制）。
func (h *UpdateHandlers) Apply(w http.ResponseWriter, r *http.Request) {
	if !h.svc.Enabled() {
		api.WriteError(w, api.ErrUpdateDisabled)
		return
	}
	res, err := h.svc.Apply(r.Context())
	if err != nil {
		api.WriteError(w, api.NewError("update_failed", err.Error(), http.StatusInternalServerError))
		return
	}
	admin, _ := AdminFrom(r.Context())
	h.record(r, admin.ID, model.ActionUpdateApplied, map[string]any{
		"version": res.Version, "replaced": res.Replaced, "needs_manual": res.NeedsManual,
	})
	api.WriteSuccess(w, http.StatusOK, res)
}

// record 写一条系统审计（不含任何 secret）。
func (h *UpdateHandlers) record(r *http.Request, adminID, action string, metadata map[string]any) {
	h.audit.Record(r.Context(), audit.Entry{
		ActorType: model.ActorTypeAdmin,
		ActorID:   adminID,
		Action:    action,
		IP:        audit.ClientIP(r),
		UserAgent: r.UserAgent(),
		RequestID: audit.RequestIDFrom(r.Context()),
		Summary:   action,
		Metadata:  metadata,
	})
}
