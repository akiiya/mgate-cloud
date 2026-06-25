import { forwardRef, type InputHTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

type InputProps = InputHTMLAttributes<HTMLInputElement>

/** 统一文本输入框，含聚焦环与禁用态样式。 */
export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, ...props }, ref) => {
    return (
      <input
        ref={ref}
        className={cn(
          'flex h-11 w-full rounded-md border border-input bg-card px-3.5 text-sm',
          'placeholder:text-muted-foreground',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:border-ring',
          'disabled:cursor-not-allowed disabled:opacity-60',
          'transition-colors',
          className,
        )}
        {...props}
      />
    )
  },
)
Input.displayName = 'Input'
