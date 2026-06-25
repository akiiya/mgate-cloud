import { getCsrfToken, setCsrfToken } from '@/lib/csrf'

/** API 统一前缀。前端所有请求都走 /api/...，与后端契约一致。 */
const BASE_PATH = '/api'

/** 后端统一响应信封。 */
interface Envelope<T> {
  ok: boolean
  data?: T
  error?: { code: string; message: string }
}

/**
 * ApiError 是前端统一的错误类型，携带稳定的 code 与可读 message。
 * 组件无需解析原始响应，只需 catch ApiError 即可做分支或提示。
 */
export class ApiError extends Error {
  readonly code: string
  readonly status: number

  constructor(code: string, message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
  }
}

/** 安全方法无需 CSRF 令牌。 */
function isStateChanging(method: string): boolean {
  return method !== 'GET' && method !== 'HEAD'
}

/**
 * ensureCsrf 确保已持有 CSRF 令牌：缓存命中直接返回，否则向后端拉取一次。
 * 拉取的同时后端会种下 csrf cookie，与请求头共同完成双提交校验。
 */
async function ensureCsrf(): Promise<string> {
  const cached = getCsrfToken()
  if (cached) return cached

  const res = await fetch(`${BASE_PATH}/auth/csrf`, { credentials: 'include' })
  const json = (await res.json()) as Envelope<{ csrfToken: string }>
  if (!res.ok || !json.ok || !json.data) {
    throw new ApiError('csrf_fetch_failed', '获取 CSRF 令牌失败', res.status)
  }
  setCsrfToken(json.data.csrfToken)
  return json.data.csrfToken
}

/**
 * request 是所有 API 调用的唯一出口：统一处理 cookie 携带、CSRF 头、
 * JSON 编解码与错误映射。请求逻辑集中于此，组件层不再各自拼 fetch。
 */
export async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (isStateChanging(method)) headers['X-CSRF-Token'] = await ensureCsrf()

  const res = await fetch(`${BASE_PATH}${path}`, {
    method,
    headers,
    credentials: 'include', // 始终携带 cookie（会话 + CSRF）
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  // 部分响应（如 204/登出）可能无 JSON 体，做容错解析。
  const json = (await res.json().catch(() => null)) as Envelope<T> | null

  if (!res.ok || !json?.ok) {
    // CSRF 失效时清空缓存，下次写请求会重新获取，避免反复失败。
    if (res.status === 403) setCsrfToken(null)
    const code = json?.error?.code ?? 'unknown_error'
    const message = json?.error?.message ?? '请求失败，请稍后重试'
    throw new ApiError(code, message, res.status)
  }

  return json.data as T
}
