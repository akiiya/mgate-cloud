import { request } from './client'
import { setCsrfToken } from '@/lib/csrf'

/** 管理员信息（与后端 /api/auth/me、/api/auth/login 返回结构一致）。 */
export interface AdminUser {
  id: string
  username: string
}

/** 获取当前登录管理员；未登录时 request 会抛出 401 ApiError。 */
export function getMe(): Promise<AdminUser> {
  return request<AdminUser>('GET', '/auth/me')
}

/** 登录：成功后后端下发会话 cookie，返回管理员信息。 */
export function login(username: string, password: string): Promise<AdminUser> {
  return request<AdminUser>('POST', '/auth/login', { username, password })
}

/** 登出：吊销会话并清除 cookie。 */
export async function logout(): Promise<void> {
  await request<null>('POST', '/auth/logout')
  // 会话结束后，旧的 CSRF 令牌不再需要，清空缓存保持干净状态。
  setCsrfToken(null)
}
