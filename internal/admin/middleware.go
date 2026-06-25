// Package admin 提供管理员相关的 HTTP 处理器与中间件。
//
// 分层约定：本包只做 HTTP 适配（解析请求、调用 service、写响应），
// 业务逻辑位于 internal/auth；二者边界清晰，handler 保持瘦身。
package admin

import (
	"context"
	"net/http"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/auth"
	"mgate-cloud/internal/model"
)

// ctxKey 是本包私有的 context key 类型。
type ctxKey int

const adminKey ctxKey = iota

// RequireAuth 返回一个鉴权中间件：校验会话 cookie，通过则把当前管理员注入 context。
//
// 设计为"中间件工厂"是为了注入 auth.Service 依赖，而不依赖包级全局变量，便于测试。
func RequireAuth(authService *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(auth.SessionCookieName)
			if err != nil || cookie.Value == "" {
				api.WriteError(w, api.ErrUnauthorized)
				return
			}

			admin, err := authService.ResolveSession(r.Context(), cookie.Value)
			if err != nil {
				api.WriteError(w, err)
				return
			}

			ctx := context.WithValue(r.Context(), adminKey, admin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminFrom 从 context 取出已认证的管理员；ok 为 false 表示未经过 RequireAuth。
func AdminFrom(ctx context.Context) (model.Admin, bool) {
	admin, ok := ctx.Value(adminKey).(model.Admin)
	return admin, ok
}

// RequireCSRF 中间件对"改变状态的请求"强制校验 CSRF 令牌（双提交模式）。
//
// 仅对写方法（POST/PUT/PATCH/DELETE）校验；GET 等安全方法天然幂等、无副作用，无需校验。
// 校验失败统一返回 403 csrf_failed。
func RequireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isStateChanging(r.Method) && !auth.VerifyCSRF(r) {
			api.WriteError(w, api.ErrCSRF)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isStateChanging 判断 HTTP 方法是否可能改变服务端状态。
func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
