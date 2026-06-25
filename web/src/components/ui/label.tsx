import { type LabelHTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

/** 表单标签：与输入框配套的统一字号与间距。 */
export function Label({ className, ...props }: LabelHTMLAttributes<HTMLLabelElement>) {
  return (
    <label
      className={cn('block text-sm font-medium text-foreground', className)}
      {...props}
    />
  )
}
