package auth

import (
	"crypto/subtle"
	"net/http"
)

// CSRF 采用"双提交 Cookie"（double-submit cookie）模式：
//
//  1. GET /api/auth/csrf 下发一个随机令牌，同时写入 cookie 并在响应体返回。
//  2. 前端发起写操作（POST）时，把令牌放入 X-CSRF-Token 请求头。
//  3. 服务端校验"请求头令牌"与"cookie 令牌"是否一致。
//
// 安全依据：跨站攻击者无法读取受害者源站的 cookie，也无法在跨站请求上
// 设置自定义请求头（受 CORS 限制），因此无法构造出"头与 cookie 一致"的请求。
// 该模式无需服务端存储 CSRF 状态，登录前（尚无会话）也能使用。

const (
	// CSRFCookieName 是承载 CSRF 令牌的 cookie 名。
	// 注意：它必须可被前端 JS 读取（HttpOnly=false），以便回填到请求头；
	// 这与会话 cookie（HttpOnly=true）的安全取舍不同，且不削弱本方案的安全性。
	CSRFCookieName = "mgate_csrf"
	// CSRFHeaderName 是前端回传 CSRF 令牌的请求头名。
	CSRFHeaderName = "X-CSRF-Token"
)

// IssueCSRFToken 生成一个新的 CSRF 令牌。
func IssueCSRFToken() (string, error) {
	return generateToken()
}

// VerifyCSRF 校验请求是否携带了一致的 CSRF 令牌（头与 cookie 双提交比对）。
//
// 使用 subtle.ConstantTimeCompare 做恒定时间比较，避免计时侧信道。
func VerifyCSRF(r *http.Request) bool {
	header := r.Header.Get(CSRFHeaderName)
	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil || header == "" || cookie.Value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) == 1
}
