package audit

import (
	"context"
	"net"
	"net/http"

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

// ClientIP 返回客户端 IP。
//
// 优先取 RealIP 中间件注入 context 的结果（按可信代理策略计算）；若未经中间件
// （如单元测试直接调用），保守地仅用 RemoteAddr，绝不信任请求头。
func ClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey).(string); ok && ip != "" {
		return ip
	}
	return remoteAddrIP(r)
}

// RealIP 中间件按可信代理策略计算客户端 IP 并注入 context。
//
// 安全要点：只有 trustProxy=true（确实部署于可信反代之后）时才采纳请求头中的来源 IP，
// 否则任何客户端都能伪造 X-Forwarded-For / CF-Connecting-IP 来污染审计。
// 信任时优先 CF-Connecting-IP（Cloudflare 单值，不可被客户端追加），其次 X-Forwarded-For 首段。
func RealIP(trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := resolveClientIP(r, trustProxy)
			ctx := context.WithValue(r.Context(), clientIPKey, ip)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// resolveClientIP 依据可信代理策略计算客户端 IP。
func resolveClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if cf := trimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
			return cf
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// "client, proxy1, proxy2"，取第一段（最初客户端）。
			if comma := indexByte(xff, ','); comma >= 0 {
				return trimSpace(xff[:comma])
			}
			return trimSpace(xff)
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

// 以下两个小工具避免为简单操作引入 strings 包，保持本文件聚焦。
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && s[start] == ' ' {
		start++
	}
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}
