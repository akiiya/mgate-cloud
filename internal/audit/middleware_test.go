package audit

import (
	"net"
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

// TestResolveClientIP 覆盖代理感知的客户端 IP 解析（登录限流按真实 IP 封禁的基础）。
func TestResolveClientIP(t *testing.T) {
	trusted := DefaultTrustedProxies()
	extra, err := ParseTrustedProxies("203.0.113.10/32")
	if err != nil {
		t.Fatalf("解析额外可信代理失败: %v", err)
	}
	trustedPlus := append(DefaultTrustedProxies(), extra...)

	cases := []struct {
		name    string
		remote  string
		headers map[string]string
		trusted []*net.IPNet
		blanket bool
		want    string
	}{
		{
			name:   "公网直连无转发头→用对端",
			remote: "198.51.100.20:5000", trusted: trusted, want: "198.51.100.20",
		},
		{
			name:    "公网直连伪造XFF→忽略，仍用对端(防伪造)",
			remote:  "198.51.100.20:5000",
			headers: map[string]string{"X-Forwarded-For": "1.2.3.4"},
			trusted: trusted, want: "198.51.100.20",
		},
		{
			name:    "本地回环反代(Caddy)→取XFF真实客户端",
			remote:  "127.0.0.1:44444",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.7"},
			trusted: trusted, want: "203.0.113.7",
		},
		{
			name:    "Docker私有网反代→取XFF真实客户端",
			remote:  "172.17.0.1:33333",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.8"},
			trusted: trusted, want: "203.0.113.8",
		},
		{
			name:    "多跳XFF→取最右侧非可信代理",
			remote:  "127.0.0.1:1",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.7, 10.0.0.2"},
			trusted: trusted, want: "203.0.113.7",
		},
		{
			name:    "CF-Connecting-IP 优先",
			remote:  "127.0.0.1:1",
			headers: map[string]string{"CF-Connecting-IP": "198.51.100.9", "X-Forwarded-For": "1.1.1.1"},
			trusted: trusted, want: "198.51.100.9",
		},
		{
			name:    "blanket=true 公网对端也采信XFF(兼容旧行为)",
			remote:  "198.51.100.20:5000",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.7"},
			trusted: trusted, blanket: true, want: "203.0.113.7",
		},
		{
			name:    "额外配置的公网反代→采信其XFF",
			remote:  "203.0.113.10:8443",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.5"},
			trusted: trustedPlus, want: "198.51.100.5",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveClientIP(req(c.remote, c.headers), c.trusted, c.blanket)
			if got != c.want {
				t.Errorf("resolveClientIP = %q, want %q", got, c.want)
			}
		})
	}
}

// TestParseTrustedProxies 校验 CIDR / 裸 IP / none 的解析。
func TestParseTrustedProxies(t *testing.T) {
	nets, err := ParseTrustedProxies("10.0.0.0/8, 203.0.113.5, 2001:db8::/32")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(nets) != 3 {
		t.Fatalf("应解析出 3 个网段，实际 %d", len(nets))
	}
	if !ipInNets("203.0.113.5", nets) {
		t.Error("裸 IP 203.0.113.5 应命中 /32")
	}
	if n, _ := ParseTrustedProxies("none"); n != nil {
		t.Error("none 应返回空集合")
	}
	if _, err := ParseTrustedProxies("not-an-ip"); err == nil {
		t.Error("非法项应报错")
	}
}
