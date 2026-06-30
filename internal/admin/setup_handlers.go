package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/auth"
	"mgate-cloud/internal/config"
	"mgate-cloud/internal/model"
)

// maxSetupBody 限制 setup 请求体大小。
const maxSetupBody = 16 << 10

// minAdminPassword 是管理员初始口令的最小长度。
const minAdminPassword = 8

// SetupHandlers 处理"无配置启动"的首次初始化（setup mode）。
type SetupHandlers struct {
	auth  *auth.Service
	audit *audit.Service
	// done 为 false 时系统处于 setup 模式；完成后置 true（与路由守卫共享）。
	done *atomic.Bool
	// cfg 是启动时的运行配置快照，提供默认值与"运行中的 app_secret"。
	cfg config.Config
}

// NewSetupHandlers 构造 setup 处理器。
func NewSetupHandlers(authService *auth.Service, auditService *audit.Service, done *atomic.Bool, cfg config.Config) *SetupHandlers {
	return &SetupHandlers{auth: authService, audit: auditService, done: done, cfg: cfg}
}

// Status 返回 setup 状态与表单默认值，供前端预填。
func (h *SetupHandlers) Status(w http.ResponseWriter, r *http.Request) {
	api.WriteSuccess(w, http.StatusOK, map[string]any{
		"setup_required": !h.done.Load(),
		"defaults": map[string]any{
			"http_addr":            h.cfg.HTTPAddr,
			"base_url":             h.cfg.BaseURL,
			"db_path":              h.cfg.DBPath,
			"mode":                 h.cfg.Mode,
			"app_secret_generated": h.cfg.AppSecretGenerated,
		},
	})
}

// setupRequest 是 setup 完成请求体。布尔用指针以区分"未提供"。
type setupRequest struct {
	HTTPAddr             string `json:"http_addr"`
	BaseURL              string `json:"base_url"`
	DBPath               string `json:"db_path"`
	Mode                 string `json:"mode"`
	CookieSecure         *bool  `json:"cookie_secure"`
	AppSecret            string `json:"app_secret"`
	AdminUsername        string `json:"admin_username"`
	AdminPassword        string `json:"admin_password"`
	AdminPasswordConfirm string `json:"admin_password_confirm"`
}

// Complete 执行初始化：校验 → 创建管理员 → 写 config.yaml → 切换到正常模式。
func (h *SetupHandlers) Complete(w http.ResponseWriter, r *http.Request) {
	if h.done.Load() {
		api.WriteError(w, api.ErrSetupAlreadyDone)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSetupBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req setupRequest
	if err := dec.Decode(&req); err != nil {
		api.WriteError(w, api.ErrInvalidRequest)
		return
	}

	username := strings.TrimSpace(req.AdminUsername)
	if username == "" {
		username = "admin"
	}
	if len(req.AdminPassword) < minAdminPassword {
		api.WriteError(w, api.NewError("weak_password", "管理员密码至少 8 位", http.StatusBadRequest))
		return
	}
	if req.AdminPassword != req.AdminPasswordConfirm {
		api.WriteError(w, api.NewError("password_mismatch", "两次输入的密码不一致", http.StatusBadRequest))
		return
	}

	mode := normalizeSetupMode(req.Mode, h.cfg.Mode)

	// app_secret：用户留空则沿用"运行中的 app_secret"，保证进程与配置文件一致。
	appSecret := strings.TrimSpace(req.AppSecret)
	if appSecret == "" {
		appSecret = h.cfg.AppSecret
	}
	if mode == config.ModeProd && appSecret == "" {
		api.WriteError(w, api.NewError("app_secret_required", "生产模式必须提供 app_secret", http.StatusBadRequest))
		return
	}

	// 仅保存口令哈希，绝不写明文。
	hash, err := auth.HashPassword(req.AdminPassword)
	if err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}

	// 创建管理员（系统当前无管理员）。
	admin, err := h.auth.CreateInitialAdmin(r.Context(), username, hash)
	if err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}

	cookieSecure := derefBool(req.CookieSecure, strings.HasPrefix(strings.ToLower(orDefault(req.BaseURL, h.cfg.BaseURL)), "https://"))
	updateEnabled := true

	fc := &config.FileConfig{
		HTTPAddr:           orDefault(req.HTTPAddr, h.cfg.HTTPAddr),
		BaseURL:            orDefault(req.BaseURL, h.cfg.BaseURL),
		DBPath:             orDefault(req.DBPath, h.cfg.DBPath),
		Mode:               mode,
		CookieSecure:       &cookieSecure,
		AppSecret:          appSecret,
		AdminUsername:      username,
		AdminPasswordHash:  hash,
		UpdateCheckEnabled: &updateEnabled,
		UpdateChannel:      orDefault(h.cfg.UpdateChannel, "stable"),
		GitHubRepo:         orDefault(h.cfg.GitHubRepo, "akiiya/mgate-cloud"),
	}
	if err := config.SaveFile(h.cfg.ConfigPath, fc); err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}

	// 切换到正常模式（路由守卫据此放行）。
	h.done.Store(true)

	// 审计：绝不记录密码 / app_secret 明文。
	h.audit.Record(r.Context(), audit.Entry{
		ActorType:  model.ActorTypeSystem,
		ActorID:    admin.ID,
		Action:     model.ActionSetupCompleted,
		TargetType: "system",
		Summary:    "完成首次初始化",
		IP:         audit.ClientIP(r),
		UserAgent:  r.UserAgent(),
		RequestID:  audit.RequestIDFrom(r.Context()),
		Metadata:   map[string]any{"admin_username": username, "mode": mode, "config_path": h.cfg.ConfigPath},
	})

	// 运行时配置中的 mode/cookie_secure/app_secret 需重启才能完全生效。
	restartRecommended := mode != h.cfg.Mode || appSecret != h.cfg.AppSecret || cookieSecure != h.cfg.CookieSecure

	api.WriteSuccess(w, http.StatusOK, map[string]any{
		"config_path":         h.cfg.ConfigPath,
		"restart_recommended": restartRecommended,
	})
}

// normalizeSetupMode 仅接受 dev/prod/test；其它回退到运行模式。
func normalizeSetupMode(m, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case config.ModeDev:
		return config.ModeDev
	case config.ModeProd, "production":
		return config.ModeProd
	case config.ModeTest:
		return config.ModeTest
	default:
		return fallback
	}
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func derefBool(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}
