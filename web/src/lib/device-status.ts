import type { Device, DeviceStatus } from '@/api/devices'

type Tone = 'success' | 'neutral' | 'primary' | 'warning' | 'danger'

/** 最近一次 Pull 在此时长内，视为“最近 Pull 联系”。 */
const RECENT_PULL_MS = 2 * 60 * 1000

/**
 * 连接状态：区分 WS 在线 / 最近 Pull / 离线。
 *
 * 关键：online（WS 在线）是进程内长连接的瞬时状态；最近 Pull 只表示设备近期通过
 * HTTPS Pull 联系过，并非保持长连接。文案上务必区分，避免把 Pull 心跳误称为 WS 在线。
 */
export function connectionMeta(d: Device): { label: string; tone: Tone; dotClass: string } {
  if (d.online) {
    return { label: 'WS 在线', tone: 'success', dotClass: 'bg-success' }
  }
  if (d.last_pull_at && Date.now() - new Date(d.last_pull_at).getTime() < RECENT_PULL_MS) {
    return { label: '最近 Pull', tone: 'primary', dotClass: 'bg-primary' }
  }
  return { label: '离线', tone: 'neutral', dotClass: 'bg-muted-foreground/40' }
}

/** 设备状态到中文标签与徽标色调的映射，集中维护以保证全站一致。 */
const map: Record<DeviceStatus, { label: string; tone: Tone }> = {
  pending: { label: '待绑定', tone: 'warning' },
  enabled: { label: '已启用', tone: 'success' },
  disabled: { label: '已禁用', tone: 'danger' },
  deleted: { label: '已删除', tone: 'neutral' },
}

export function deviceStatusMeta(status: DeviceStatus): { label: string; tone: Tone } {
  return map[status] ?? { label: status, tone: 'neutral' }
}

/** 在线/离线徽标。online 是进程内连接的瞬时状态。 */
export function presenceMeta(online: boolean): { label: string; tone: Tone } {
  return online ? { label: '在线', tone: 'success' } : { label: '离线', tone: 'neutral' }
}
