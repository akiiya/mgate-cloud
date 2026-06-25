import { request } from './client'

/** 设备状态枚举，与后端保持一致。 */
export type DeviceStatus = 'pending' | 'enabled' | 'disabled' | 'deleted'

/** 设备对外结构（与后端 deviceDTO 对应）。 */
export interface Device {
  id: string
  name: string
  remark: string | null
  status: DeviceStatus
  /** online 是进程内 WebSocket 连接的瞬时状态，非持久化。 */
  online: boolean
  agent_version: string
  mgate_version: string
  device_model: string
  hostname: string
  firmware_info: string
  last_seen_at: string | null
  last_enrolled_at: string | null
  last_ws_connected_at: string | null
  last_ws_disconnected_at: string | null
  last_pull_at: string | null
  last_pull_status_at: string | null
  created_at: string
  updated_at: string
}

/** 设备详情：设备本体 + 在线状态 + 最近设备码状态 + 凭证摘要 + 最新状态。 */
export interface DeviceDetail {
  device: Device
  online: boolean
  pairing: {
    status: 'none' | 'active' | 'used' | 'expired'
    expires_at: string | null
  }
  credential: {
    active_count: number
  }
  /** 最新状态为设备上报的原始 JSON 对象；无上报时为 null。 */
  latest_status: unknown | null
  latest_status_reported_at: string | null
  latest_status_received_at: string | null
  /** 最新状态来源：ws（WebSocket）或 pull（HTTPS Pull）。 */
  latest_status_source: string | null
}

/** 生成设备码的结果。device_code 明文只在此返回一次。 */
export interface PairingCodeResult {
  device_code: string
  expires_at: string
}

/** 设备相关 API。请求逻辑统一走 client，自动带 cookie 与 CSRF。 */
export const deviceApi = {
  list(): Promise<{ items: Device[] }> {
    return request<{ items: Device[] }>('GET', '/admin/devices')
  },
  create(name: string, remark: string): Promise<{ device: Device }> {
    return request<{ device: Device }>('POST', '/admin/devices', { name, remark })
  },
  get(deviceId: string): Promise<DeviceDetail> {
    return request<DeviceDetail>('GET', `/admin/devices/${deviceId}`)
  },
  generatePairingCode(deviceId: string): Promise<PairingCodeResult> {
    return request<PairingCodeResult>('POST', `/admin/devices/${deviceId}/pairing-code`)
  },
  disable(deviceId: string): Promise<null> {
    return request<null>('POST', `/admin/devices/${deviceId}/disable`)
  },
  enable(deviceId: string): Promise<null> {
    return request<null>('POST', `/admin/devices/${deviceId}/enable`)
  },
}
