import { request } from './client'

/** 系统只读信息（与后端 admin.SystemInfo 对应）。 */
export interface SystemInfo {
  version: string
  mode: string
  update_channel: string
  update_enabled: boolean
  cookie_secure: boolean
}

export const systemApi = {
  get(): Promise<SystemInfo> {
    return request<SystemInfo>('GET', '/admin/system')
  },
}
