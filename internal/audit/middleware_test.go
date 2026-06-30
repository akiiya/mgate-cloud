package audit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func req(remoteAddr string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

// TestResolveClientIP 覆盖自适配的真实客户端 IP 解析（审计与登录限流的基础）。
func TestResolveClientIP(t *testing.T) {
	cases := []struct {
		name    string
		remote  string
		headers map[string]string
		want    string
	}{
		{
			name:   "直连无转发头→RemoteAddr",
			remote: "203.0.113.20:5000", want: "203.0.113.20",
		},
		{
			name:    "Cloudflare→取 CF-Connecting-IP",
			remote:  "198.51.100.1:443", // 对端是 Cloudflare 边缘
			headers: map[string]string{"CF-Connecting-IP": "203.0.113.7", "X-Forwarded-For": "203.0.113.7, 198.51.100.1"},
			want:    "203.0.113.7",
		},
		{
			name:    "反代(Caddy/Nginx)→取 XFF 最左侧",
			remote:  "127.0.0.1:44444",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.8"},
			want:    "203.0.113.8",
		},
		{
			name:    "XFF 多段→取最左侧最初客户端",
			remote:  "127.0.0.1:1",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.9, 70.0.0.1, 127.0.0.1"},
			want:    "203.0.113.9",
		},
		{
			name:    "XFF 含非法首段→跳到首个有效 IP",
			remote:  "127.0.0.1:1",
			headers: map[string]string{"X-Forwarded-For": "unknown, 203.0.113.10"},
			want:    "203.0.113.10",
		},
		{
			name:    "CF 头非法→退回 XFF",
			remote:  "127.0.0.1:1",
			headers: map[string]string{"CF-Connecting-IP": "not-an-ip", "X-Forwarded-For": "203.0.113.11"},
			want:    "203.0.113.11",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveClientIP(req(c.remote, c.headers)); got != c.want {
				t.Errorf("resolveClientIP = %q, want %q", got, c.want)
			}
		})
	}
}
