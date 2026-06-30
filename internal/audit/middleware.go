package audit

import (
	"context"
	"net"
	"net/http"
	"strings"

	"mgate-cloud/internal/util"
)

// ctxKey 是本包私有的 context key 类型，避免与其他包的 key 冲突。
type ctxKey int

const (
	requestIDKey ctxKey = iota
	clientIPKey
)

// RequestIDHeader 是回传给客户端的请求 ID 响应头。
const RequestIDHeader = "X-Request-Id"

// RequestID 中间件为每个请求生成唯一 ID，注入 context 并写入响应头。
//
// 该 ID 贯穿日志与审计（audit_logs.request_id），便于把一次请求产生的多条记录
// 串联起来排查问题。
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := util.NewID()
		w.Header().Set(RequestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFrom 从 context 取出请求 ID，不存在时返回空串。
func RequestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// ClientIP 返回客户端真实 IP。
//
// 优先取 RealIP 中间件注入 context 的结果；若未经中间件（如单元测试直接调用），
// 退回 RemoteAddr。
func ClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey).(string); ok && ip != "" {
		return ip
	}
	return remoteAddrIP(r)
}

// RealIP 中间件计算客户端真实 IP 并注入 context，自动适配不同部署环境：
//
//   - Cloudflare：取 CF-Connecting-IP（Cloudflare 注入的真实客户端，单值不可被客户端追加）。
//   - 其它反代（Caddy/Nginx 等）：取 X-Forwarded-For 最左侧（最初客户端）。
//   - 直连：取 RemoteAddr 对端地址。
//
// 这样无需任何配置即可在各环境拿到“登录者的真实来源 IP”，供审计与登录失败限流使用。
//
// 注意（部署约定）：本服务应只经反代访问、不直接暴露公网（绑定 127.0.0.1 / 私有网或防火墙限制）。
// 此前提下转发头由可信反代设置，不会被外部伪造。
func RealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), clientIPKey, resolveClientIP(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolveClientIP 按 CF-Connecting-IP → X-Forwarded-For(最左有效) → RemoteAddr 的顺序解析真实客户端 IP。
func resolveClientIP(r *http.Request) string {
	if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" && net.ParseIP(cf) != nil {
		return cf
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// "client, proxy1, proxy2"：最左侧为最初客户端，取第一个能解析的 IP。
		for _, part := range strings.Split(xff, ",") {
			ip := strings.TrimSpace(part)
			if ip != "" && net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	return remoteAddrIP(r)
}

// remoteAddrIP 从 RemoteAddr 剥离端口，得到对端 IP。
func remoteAddrIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
