import { useEffect, type ReactNode } from 'react'
import { X } from 'lucide-react'
import { cn } from '@/lib/cn'

interface DialogProps {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  children: ReactNode
  className?: string
}

/**
 * 轻量模态框。
 *
 * 刻意不引入 Radix 等依赖：Phase 2 的弹窗需求简单（创建设备、展示设备码），
 * 一个带遮罩、支持 Esc 关闭的受控组件足矣，符合"避免过度工程"。
 */
export function Dialog({ open, onClose, title, description, children, className }: DialogProps) {
  // 打开时支持 Esc 关闭，并锁定 body 滚动，关闭时还原。
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = prevOverflow
    }
  }, [open, onClose])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      {/* 遮罩：点击关闭 */}
      <div className="absolute inset-0 bg-foreground/30 backdrop-blur-sm" onClick={onClose} aria-hidden />

      {/* 内容卡片 */}
      <div
        role="dialog"
        aria-modal="true"
        className={cn(
          'relative w-full max-w-md rounded-lg border border-border bg-card p-6 shadow-soft',
          className,
        )}
      >
        <button
          onClick={onClose}
          aria-label="关闭"
          className="absolute right-4 top-4 rounded-md p-1 text-muted-foreground hover:bg-muted"
        >
          <X className="h-4 w-4" />
        </button>

        <h2 className="text-base font-semibold">{title}</h2>
        {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}

        <div className="mt-4">{children}</div>
      </div>
    </div>
  )
}
