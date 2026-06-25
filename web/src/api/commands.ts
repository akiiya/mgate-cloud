import { request } from './client'

/** 命令状态，与后端一致。 */
export type CommandStatus =
  | 'pending'
  | 'leased'
  | 'sent'
  | 'acked'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'timeout'
  | 'canceled'
  | 'expired'

/** 命令对外结构（与后端 commandDTO 对应）。 */
export interface Command {
  id: string
  device_id: string
  device_name?: string
  action: string
  params: unknown
  status: CommandStatus
  timeout_sec: number
  attempts: number
  max_attempts: number
  leased_by: string | null
  lease_until: string | null
  expires_at: string | null
  last_error: string | null
  created_at: string
  sent_at: string | null
  acked_at: string | null
  started_at: string | null
  finished_at: string | null
}

/** 命令创建后的投递提示。 */
export type DeliveryHint = 'delivered_via_ws' | 'queued_for_retry' | 'device_offline_waiting_for_pull' | string

/** 命令结果。 */
export interface CommandResult {
  status: string
  exit_code: number | null
  stdout: string
  stderr: string
  result: unknown
  error_message: string
  truncated: boolean
  started_at: string | null
  finished_at: string | null
  received_at: string
}

export interface CommandDetail {
  command: Command
  result: CommandResult | null
}

export interface CommandListQuery {
  device_id?: string
  status?: string
  limit?: number
}

/** 命令相关 API。 */
export const commandApi = {
  list(query: CommandListQuery = {}): Promise<{ items: Command[] }> {
    const params = new URLSearchParams()
    if (query.device_id) params.set('device_id', query.device_id)
    if (query.status) params.set('status', query.status)
    if (query.limit) params.set('limit', String(query.limit))
    const qs = params.toString()
    return request<{ items: Command[] }>('GET', `/admin/commands${qs ? `?${qs}` : ''}`)
  },
  get(commandId: string): Promise<CommandDetail> {
    return request<CommandDetail>('GET', `/admin/commands/${commandId}`)
  },
  create(
    deviceId: string,
    action: string,
    params: Record<string, unknown>,
    timeoutSec?: number,
  ): Promise<{ command: Command; delivery_hint: DeliveryHint }> {
    return request<{ command: Command; delivery_hint: DeliveryHint }>(
      'POST',
      `/admin/devices/${deviceId}/commands`,
      { action, params, timeout_sec: timeoutSec ?? 0 },
    )
  },
  cancel(commandId: string): Promise<null> {
    return request<null>('POST', `/admin/commands/${commandId}/cancel`)
  },
}

/** 终态判断：终态命令不可取消。 */
export function isTerminalStatus(status: CommandStatus): boolean {
  return ['succeeded', 'failed', 'timeout', 'canceled', 'expired'].includes(status)
}
