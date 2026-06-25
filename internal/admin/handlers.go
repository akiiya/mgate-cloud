package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/auth"
	"mgate-cloud/internal/model"
)

// Handlers 聚合管理员相关 HTTP 处理器及其依赖。
type Handlers struct {
	auth         *auth.Service
	audit        *audit.Service
	cookieSecure bool
	sessionTTL   time.Duration
}

// NewHandlers 构造 Handlers，显式注入依赖，避免全局状态。
func NewHandlers(authService *auth.Service, auditService *audit.Service, cookieSecure bool, sessionTTL time.Duration) *Handlers {
	return &Handlers{
		auth:         authService,
		audit:        auditService,
		cookieSecure: cookieSecure,
		sessionTTL:   sessionTTL,
	}
}

// loginRequest 是登录请求体。字段标签固定 JSON 契约。
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Healthz 健康检查：始终返回运行中。供存活探测与前端连通性判断使用。
func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	api.WriteSuccess(w, http.StatusOK, map[string]any{"status": "ok"})
}

// CSRF 下发 CSRF 令牌：写入可被 JS 读取的 cookie，并在响应体返回供前端回填请求头。
func (h *Handlers) CSRF(w http.ResponseWriter, r *http.Request) {
	token, err := auth.IssueCSRFToken()
	if err != nil {
		api.WriteError(w, err)
		return
	}
	h.setCSRFCookie(w, token)
	api.WriteSuccess(w, http.StatusOK, map[string]any{"csrfToken": token})
}

// Me 返回当前登录管理员信息。须经 RequireAuth 中间件，未登录由中间件返回 401。
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	admin, ok := AdminFrom(r.Context())
	if !ok {
		// 正常流程下不会到达：RequireAuth 已保证存在；防御性返回未授权。
		api.WriteError(w, api.ErrUnauthorized)
		return
	}
	api.WriteSuccess(w, http.StatusOK, map[string]any{
		"id":       admin.ID,
		"username": admin.Username,
	})
}

// Login 处理管理员登录。
//
// 成功：下发会话 cookie，记录 admin.login.success；
// 失败：返回统一凭据错误（不区分用户名/口令），记录 admin.login.failed。
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		api.WriteError(w, api.ErrBadRequest)
		return
	}

	ip := audit.ClientIP(r)
	userAgent := r.UserAgent()
	requestID := audit.RequestIDFrom(r.Context())

	token, admin, err := h.auth.Login(r.Context(), req.Username, req.Password, userAgent, ip)
	if err != nil {
		// 登录失败审计：actor_id 留空（主体未确认），用户名入 metadata（非敏感）。
		h.audit.Record(r.Context(), audit.Entry{
			ActorType: model.ActorTypeAdmin,
			Action:    model.ActionLoginFailed,
			IP:        ip,
			UserAgent: userAgent,
			RequestID: requestID,
			Summary:   "管理员登录失败",
			Metadata:  map[string]any{"username": req.Username},
		})
		api.WriteError(w, err)
		return
	}

	h.setSessionCookie(w, token)
	h.audit.Record(r.Context(), audit.Entry{
		ActorType: model.ActorTypeAdmin,
		ActorID:   admin.ID,
		Action:    model.ActionLoginSuccess,
		IP:        ip,
		UserAgent: userAgent,
		RequestID: requestID,
		Summary:   "管理员登录成功",
		Metadata:  map[string]any{"username": admin.Username},
	})

	api.WriteSuccess(w, http.StatusOK, map[string]any{
		"id":       admin.ID,
		"username": admin.Username,
	})
}

// Logout 处理登出：吊销会话、清除 cookie 并记录审计。须经 RequireAuth。
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	admin, _ := AdminFrom(r.Context())

	// RequireAuth 已确保 cookie 存在；此处再次读取以拿到原始令牌用于吊销。
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
		if err := h.auth.Logout(r.Context(), cookie.Value); err != nil {
			api.WriteError(w, err)
			return
		}
	}

	h.clearSessionCookie(w)
	h.audit.Record(r.Context(), audit.Entry{
		ActorType: model.ActorTypeAdmin,
		ActorID:   admin.ID,
		Action:    model.ActionLogout,
		IP:        audit.ClientIP(r),
		UserAgent: r.UserAgent(),
		RequestID: audit.RequestIDFrom(r.Context()),
		Summary:   "管理员登出",
	})

	api.WriteSuccess(w, http.StatusOK, nil)
}

// --- cookie 辅助方法：集中 cookie 安全属性，避免各处重复且易漏配 ---

// setSessionCookie 下发会话 cookie。
//
// 安全属性：HttpOnly（防 JS 读取，缓解 XSS 窃取）、SameSite=Lax（缓解 CSRF）、
// Secure 由配置控制（公网 https 必开）。
func (h *Handlers) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.sessionTTL.Seconds()),
	})
}

// clearSessionCookie 通过设置 MaxAge<0 立即失效会话 cookie。
func (h *Handlers) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// setCSRFCookie 下发 CSRF cookie。
//
// 与会话 cookie 不同，CSRF cookie 必须允许 JS 读取（HttpOnly=false），
// 以便前端回填到 X-CSRF-Token 请求头完成双提交校验。
func (h *Handlers) setCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.sessionTTL.Seconds()),
	})
}
