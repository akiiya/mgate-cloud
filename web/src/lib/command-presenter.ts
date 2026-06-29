import type { Command, CommandResult, CommandStatus } from '@/api/commands'
import { getAction } from './action-catalog'

/**
 * 命令结果「人话」翻译。
 *
 * 把 (action + status + result) 翻译成普通用户能看懂的一句话与处置建议，
 * 而不是直接抛 stdout / stderr / result_json。原始字段仍保留在页面的「高级详情」里，
 * 供需要排障的工程用户展开查看。
 */

export type Severity = 'success' | 'warning' | 'error' | 'info'

export interface CommandPresentation {
  /** 人类可读的动作标题。 */
  title: string
  /** 一句话概述发生了什么。 */
  summary: string
  /** 严重级别，驱动配色与图标。 */
  severity: Severity
  /** 可选的处置建议（下一步该做什么）。 */
  suggestedAction?: string
}

/** 进行中的状态文案。 */
const inFlight: Partial<Record<CommandStatus, string>> = {
  pending: '已创建，等待投递给设备。',
  leased: '正在投递给设备。',
  sent: '已发送，等待设备确认。',
  acked: '设备已确认，正在执行。',
  running: '设备正在执行。',
}

/** 把后端的错误原因压成简短人话；过滤掉过于工程化的内容。 */
function humanizeError(reason: string): string {
  const r = reason.trim()
  if (!r) return ''
  // 控制长度，避免把整段堆栈塞进提示。
  return r.length > 160 ? r.slice(0, 160) + '…' : r
}

/**
 * 生成命令的产品化呈现。
 * @param command 命令本体
 * @param result  执行结果（可能为空）
 */
export function presentCommand(command: Command, result?: CommandResult | null): CommandPresentation {
  const entry = getAction(command.action)
  const title = entry.title
  const status = command.status

  // 进行中：按状态给出过程文案。
  if (inFlight[status]) {
    return {
      title,
      summary: inFlight[status]!,
      severity: 'info',
      suggestedAction:
        status === 'pending' ? '若设备长时间离线，命令会在过期时间后自动失效。' : undefined,
    }
  }

  switch (status) {
    case 'succeeded':
      return { title, summary: entry.successHint, severity: 'success' }

    case 'failed': {
      const reason = humanizeError(result?.error_message || command.last_error || '')
      return {
        title,
        summary: reason ? `${entry.failureHint}原因：${reason}` : `${entry.failureHint}设备未给出更多信息。`,
        severity: 'error',
        suggestedAction: '确认设备状态后可重新下发该操作。',
      }
    }

    case 'timeout':
      return {
        title,
        summary: '设备未在限定时间内响应，操作判定为超时。',
        severity: 'error',
        suggestedAction: '检查设备是否在线、网络是否稳定，然后重试。',
      }

    case 'expired':
      return {
        title,
        summary: '命令在设备连接前已过期，未被执行。',
        severity: 'warning',
        suggestedAction: '设备恢复在线后重新下发。',
      }

    case 'canceled':
      return {
        title,
        summary: '操作已被取消。',
        severity: 'warning',
        suggestedAction: '若仍需执行，可重新下发。',
      }

    default:
      return { title, summary: '操作状态未知。', severity: 'info' }
  }
}

/** 严重级别 → 徽标色调，复用统一 Badge 的语义色。 */
export function severityTone(s: Severity): 'success' | 'warning' | 'danger' | 'info' {
  switch (s) {
    case 'success':
      return 'success'
    case 'warning':
      return 'warning'
    case 'error':
      return 'danger'
    default:
      return 'info'
  }
}
