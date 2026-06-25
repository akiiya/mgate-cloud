import { request } from './client'

/** 检查更新结果。 */
export interface UpdateCheck {
  current_version: string
  latest_version: string
  has_update: boolean
  published_at: string
  html_url: string
  asset_name: string
  asset_available: boolean
  channel: string
}

/** 自更新结果。 */
export interface UpdateApplyResult {
  version: string
  replaced: boolean
  needs_manual: boolean
  needs_restart: boolean
  staged_path?: string
  backup_path?: string
  message: string
}

export const updateApi = {
  check(): Promise<UpdateCheck> {
    return request<UpdateCheck>('GET', '/admin/update/check')
  },
  apply(): Promise<UpdateApplyResult> {
    return request<UpdateApplyResult>('POST', '/admin/update/apply')
  },
}
