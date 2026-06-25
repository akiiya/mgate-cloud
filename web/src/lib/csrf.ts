/**
 * CSRF 令牌的内存缓存。
 *
 * 双提交模式下，前端需把令牌放入 X-CSRF-Token 请求头。令牌随 GET /api/auth/csrf
 * 下发（同时写入 cookie）。这里在内存缓存令牌，避免每次写请求都额外往返一次；
 * 当令牌失效（403）时由 client 清空缓存以触发重新获取。
 */
let token: string | null = null

export function getCsrfToken(): string | null {
  return token
}

export function setCsrfToken(value: string | null): void {
  token = value
}
