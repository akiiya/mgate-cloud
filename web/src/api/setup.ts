import { ApiError } from './client'

const BASE_PATH = '/api'

/** Setup 状态与表单默认值。 */
export interface SetupStatus {
  setup_required: boolean
  defaults: {
    http_addr: string
    base_url: string
    db_path: string
    mode: string
    app_secret_generated: boolean
  }
}

/** 完成 Setup 的请求体。 */
export interface SetupPayload {
  admin_username: string
  admin_password: string
  admin_password_confirm: string
  mode: string
  base_url: string
  http_addr?: string
  db_path?: string
  app_secret?: string
}

export interface SetupResult {
  config_path: string
  restart_recommended: boolean
}

// setup 接口在 setup 模式下可直接访问，且无需 CSRF（无会话、由服务端 setupDone 守卫）。
// 因此使用独立的最小 fetch，不走带 CSRF 的通用 client。
async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${BASE_PATH}${path}`, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
    credentials: 'include',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  const json = (await res.json().catch(() => null)) as { ok: boolean; data?: T; error?: { code: string; message: string } } | null
  if (!res.ok || !json?.ok) {
    throw new ApiError(json?.error?.code ?? 'unknown', json?.error?.message ?? '请求失败', res.status)
  }
  return json.data as T
}

export function getSetupStatus(): Promise<SetupStatus> {
  return call<SetupStatus>('GET', '/setup/status')
}

export function completeSetup(payload: SetupPayload): Promise<SetupResult> {
  return call<SetupResult>('POST', '/setup/complete', payload)
}
