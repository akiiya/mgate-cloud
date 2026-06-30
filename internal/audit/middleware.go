package audit

import (
	"context"
	"fmt"
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
// 安全要点：仅当“直接对端”（RemoteAddr）本身是可信代理时，才采信其转发头里的来源 IP；
// 否则（公网直连客户端）一律以对端地址为准，杜绝客户端伪造 X-Forwarded-For / CF-Connecting-IP。
// 这对登录失败限流尤其重要：必须按“真实客户端 IP”封禁，而不是把所有人归并到反代的 IP。
//
//   - trusted：可信代理网段（默认本地回环 + 私有/链路本地，可经 MGATE_TRUSTED_PROXIES 追加）。
//   - blanket：兼容旧 MGATE_TRUST_PROXY_HEADERS=true 的“无条件信任任意对端转发头”行为。
func RealIP(trusted []*net.IPNet, blanket bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := resolveClientIP(r, trusted, blanket)
			ctx := context.WithValue(r.Context(), clientIPKey, ip)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// resolveClientIP 依据可信代理策略计算客户端 IP。
func resolveClientIP(r *http.Request, trusted []*net.IPNet, blanket bool) string {
	peer := remoteAddrIP(r)
	// 只有“对端是可信代理”或开启 blanket 时，才从转发头还原真实客户端。
	if blanket || ipInNets(peer, trusted) {
		// Cloudflare 单值头，不可被客户端追加，优先采用。
		if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
			return cf
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			// 从右向左取第一个“非可信代理”的地址：最接近可信边界、最难被客户端伪造。
			for i := len(parts) - 1; i >= 0; i-- {
				c := strings.TrimSpace(parts[i])
				if c != "" && !ipInNets(c, trusted) {
					return c
				}
			}
			// 链路上全是可信代理：退回最左（最初客户端）。
			if first := strings.TrimSpace(parts[0]); first != "" {
				return first
			}
		}
	}
	return peer
}

// remoteAddrIP 从 RemoteAddr 剥离端口，得到对端 IP。
func remoteAddrIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// ipInNets 报告 ip 是否落在任一网段内。
func ipInNets(ip string, nets []*net.IPNet) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	for _, n := range nets {
		if n != nil && n.Contains(parsed) {
			return true
		}
	}
	return false
}

// DefaultTrustedProxies 返回默认可信代理网段：本地回环 + 私有 / 链路本地地址。
//
// 这些地址不可能来自公网直连，因此采信来自它们的转发头是安全的；
// 同时让“反代（Caddy/Nginx/Docker 网络）”部署开箱即用——无需手工配置即可识别真实客户端 IP。
func DefaultTrustedProxies() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8", "::1/128", // 回环
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", // RFC1918 私有
		"169.254.0.0/16", "fe80::/10", // 链路本地
		"fc00::/7", // IPv6 唯一本地地址（ULA）
	}
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// ParseTrustedProxies 解析逗号分隔的 CIDR / IP 列表为网段集合。
//
// 裸 IP 视为 /32（IPv4）或 /128（IPv6）。空串、或特殊值 "none"/"off" 返回空集合。
func ParseTrustedProxies(list string) ([]*net.IPNet, error) {
	list = strings.TrimSpace(list)
	if list == "" || strings.EqualFold(list, "none") || strings.EqualFold(list, "off") {
		return nil, nil
	}
	var out []*net.IPNet
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if !strings.Contains(item, "/") {
			if ip := net.ParseIP(item); ip != nil {
				if ip.To4() != nil {
					item += "/32"
				} else {
					item += "/128"
				}
			}
		}
		_, n, err := net.ParseCIDR(item)
		if err != nil {
			return nil, fmt.Errorf("audit: 无效的可信代理项 %q: %w", item, err)
		}
		out = append(out, n)
	}
	return out, nil
}
