import { forwardRef, type ButtonHTMLAttributes } from 'react'
import { Loader2 } from 'lucide-react'
import { cn } from '@/lib/cn'

type Variant = 'primary' | 'outline' | 'ghost'
type Size = 'default' | 'sm'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
  /** loading 为真时显示加载图标并禁用按钮，避免重复提交。 */
  loading?: boolean
}

const variantClasses: Record<Variant, string> = {
  primary:
    'bg-primary text-primary-foreground shadow-soft hover:bg-primary/90 focus-visible:ring-ring',
  outline:
    'border border-input bg-card hover:bg-muted focus-visible:ring-ring',
  ghost: 'hover:bg-muted focus-visible:ring-ring',
}

const sizeClasses: Record<Size, string> = {
  default: 'h-11 px-5 text-sm',
  sm: 'h-9 px-3 text-sm',
}

/** 统一的按钮组件：受控的变体、尺寸与 loading 态，保证全站按钮一致。 */
export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'primary', size = 'default', loading = false, disabled, children, ...props }, ref) => {
    return (
      <button
        ref={ref}
        disabled={disabled || loading}
        className={cn(
          'inline-flex items-center justify-center gap-2 rounded-md font-medium transition-colors',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-background',
          'disabled:pointer-events-none disabled:opacity-60',
          variantClasses[variant],
          sizeClasses[size],
          className,
        )}
        {...props}
      >
        {loading && <Loader2 className="h-4 w-4 animate-spin" aria-hidden />}
        {children}
      </button>
    )
  },
)
Button.displayName = 'Button'
