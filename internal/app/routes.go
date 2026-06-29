package app

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync/atomic"

	"mgate-cloud/internal/admin"
	"mgate-cloud/internal/agent"
	"mgate-cloud/internal/api"
	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/auth"
	"mgate-cloud/internal/webui"
)

// routeDeps 汇集装配路由所需的处理器与依赖，避免 buildRoutes 参数过长。
type routeDeps struct {
	auth       *admin.Handlers
	devices    *admin.DeviceHandlers
	commands   *admin.CommandHandlers
	setup      *admin.SetupHandlers
	update     *admin.UpdateHandlers
	system     *admin.SystemHandlers
	agent      *agent.Handlers
	ws         *agent.WSHandlers
	pull       *agent.PullHandlers
	authSvc    *auth.Service
	distFS     fs.FS
	trustProxy bool
	setupDone  *atomic.Bool
	readyCheck func(ctx context.Context) error // 就绪探测（DB 可用即就绪）
}

// buildRoutes 装配全部路由与中间件，返回顶层 HTTP 处理器。
//
// 路由分层：
//   - "/api/admin/..."：后台接口，需登录；写操作另需 CSRF。
//   - "/api/agent/..."：设备 agent 公开接口，无 session、无 CSRF，靠设备码鉴权。
//   - "/api/auth/..."、"/api/healthz"：认证与健康检查。
//   - "/"：内嵌 SPA；因 "/api/" 前缀更具体，API 不会被 SPA 回退吞掉。
func buildRoutes(d routeDeps) http.Handler {
	requireAuth := admin.RequireAuth(d.authSvc)

	// authed 组合"需登录"；authedWrite 额外叠加 CSRF（用于写操作）。
	authed := func(h http.HandlerFunc) http.Handler { return requireAuth(h) }
	authedWrite := func(h http.HandlerFunc) http.Handler { return admin.RequireCSRF(requireAuth(h)) }

	apiMux := http.NewServeMux()

	// 健康检查与就绪探测：无需登录、无需 CSRF（均为安全的 GET）。
	// healthz：进程存活；readyz：依赖（数据库）可用才就绪。
	apiMux.HandleFunc("GET /api/healthz", d.auth.Healthz)
	apiMux.HandleFunc("GET /api/readyz", readyzHandler(d.readyCheck))
	apiMux.HandleFunc("GET /api/auth/csrf", d.auth.CSRF)

	// 首次初始化（setup mode）：始终可访问，由 setup 守卫白名单放行。
	apiMux.HandleFunc("GET /api/setup/status", d.setup.Status)
	apiMux.HandleFunc("POST /api/setup/complete", d.setup.Complete)

	// 认证。
	apiMux.Handle("POST /api/auth/login", admin.RequireCSRF(http.HandlerFunc(d.auth.Login)))
	apiMux.Handle("GET /api/auth/me", authed(d.auth.Me))
	apiMux.Handle("POST /api/auth/logout", authedWrite(d.auth.Logout))

	// 管理员设备接口：GET 需登录；POST 需登录 + CSRF。
	apiMux.Handle("GET /api/admin/devices", authed(d.devices.List))
	apiMux.Handle("POST /api/admin/devices", authedWrite(d.devices.Create))
	apiMux.Handle("GET /api/admin/devices/{device_id}", authed(d.devices.Detail))
	apiMux.Handle("POST /api/admin/devices/{device_id}/pairing-code", authedWrite(d.devices.PairingCode))
	apiMux.Handle("POST /api/admin/devices/{device_id}/disable", authedWrite(d.devices.Disable))
	apiMux.Handle("POST /api/admin/devices/{device_id}/enable", authedWrite(d.devices.Enable))

	// 管理员命令接口：GET 需登录；写操作需登录 + CSRF。
	apiMux.Handle("GET /api/admin/commands", authed(d.commands.List))
	apiMux.Handle("GET /api/admin/commands/{command_id}", authed(d.commands.Detail))
	apiMux.Handle("POST /api/admin/devices/{device_id}/commands", authedWrite(d.commands.Create))
	apiMux.Handle("POST /api/admin/commands/{command_id}/cancel", authedWrite(d.commands.Cancel))

	// 更新：检查需登录；应用需登录 + CSRF。
	apiMux.Handle("GET /api/admin/update/check", authed(d.update.Check))
	apiMux.Handle("POST /api/admin/update/apply", authedWrite(d.update.Apply))

	// 系统信息（只读）：版本 / 运行模式 / 更新通道，供「设置」页展示。
	apiMux.Handle("GET /api/admin/system", authed(d.system.Info))

	// 设备 agent 公开接口：enroll（无 session/CSRF）与 WebSocket 接入（bearer token 鉴权）。
	apiMux.HandleFunc("POST /api/agent/enroll", d.agent.Enroll)
	apiMux.HandleFunc("GET /api/agent/ws", d.ws.ServeWS)
	apiMux.HandleFunc("POST /api/agent/pull", d.pull.Pull)

	root := http.NewServeMux()
	// setup 守卫只包裹 /api/*；静态 SPA 始终可访问（以便加载 /#/setup 页面）。
	root.Handle("/api/", setupGuard(d.setupDone, apiMux))
	root.Handle("/", webui.NewSPAHandler(d.distFS))

	// 中间件自外向内：请求 ID → 客户端 IP（按可信代理策略）→ panic 恢复 → 业务路由。
	return audit.RequestID(audit.RealIP(d.trustProxy)(recoverMiddleware(root)))
}

// setupGuard 在 setup 未完成时，仅放行 healthz/readyz/setup 接口，其余 /api/* 返回 setup_required。
//
// setupDone 为共享原子标志：setup 完成后翻转为 true，无需重启即可放行后续请求。
func setupGuard(setupDone *atomic.Bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if setupDone == nil || setupDone.Load() {
			next.ServeHTTP(w, r)
			return
		}
		p := r.URL.Path
		if p == "/api/healthz" || p == "/api/readyz" || strings.HasPrefix(p, "/api/setup/") {
			next.ServeHTTP(w, r)
			return
		}
		api.WriteError(w, api.ErrSetupRequired)
	})
}

// readyzHandler 返回就绪探测处理器：依赖检查通过返回 200，否则 503。
func readyzHandler(check func(ctx context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			if err := check(r.Context()); err != nil {
				log.Printf("app: readyz 失败: %v", err)
				api.WriteError(w, api.ErrNotReady)
				return
			}
		}
		api.WriteSuccess(w, http.StatusOK, map[string]any{"status": "ready"})
	}
}

// recoverMiddleware 捕获 handler 中的 panic，避免单个请求崩溃拖垮整个进程。
//
// 捕获后记录日志并返回统一的内部错误，绝不向客户端泄露堆栈细节。
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("app: 请求处理发生 panic: %v", rec)
				api.WriteError(w, api.ErrInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
