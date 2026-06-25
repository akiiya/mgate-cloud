import { AlertCircle } from 'lucide-react'
import { cn } from '@/lib/cn'

interface AlertProps {
  message: string
  className?: string
}

/**
 * 统一错误提示。全站的错误展示都走此组件，保证视觉与语义一致，
 * 避免每个页面各写一套提示样式。
 */
export function ErrorAlert({ message, className }: AlertProps) {
  return (
    <div
      role="alert"
      className={cn(
        'flex items-start gap-2.5 rounded-md border border-destructive/20 bg-destructive/5 px-3.5 py-3 text-sm text-destructive',
        className,
      )}
    >
      <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
      <span>{message}</span>
    </div>
  )
}
