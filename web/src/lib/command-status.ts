import type { CommandStatus } from '@/api/commands'

type Tone = 'success' | 'neutral' | 'primary' | 'warning' | 'danger'

/** 命令状态到中文标签与徽标色调的映射，集中维护以保证全站一致。 */
const map: Record<CommandStatus, { label: string; tone: Tone }> = {
  pending: { label: '待投递', tone: 'warning' },
  leased: { label: '投递中', tone: 'warning' },
  sent: { label: '已投递', tone: 'primary' },
  acked: { label: '已确认', tone: 'primary' },
  running: { label: '执行中', tone: 'primary' },
  succeeded: { label: '成功', tone: 'success' },
  failed: { label: '失败', tone: 'danger' },
  timeout: { label: '超时', tone: 'danger' },
  canceled: { label: '已取消', tone: 'neutral' },
  expired: { label: '已过期', tone: 'neutral' },
}

export function commandStatusMeta(status: CommandStatus): { label: string; tone: Tone } {
  return map[status] ?? { label: status, tone: 'neutral' }
}
