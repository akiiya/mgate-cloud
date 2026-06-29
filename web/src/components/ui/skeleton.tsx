import { type HTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

/** 骨架占位块：加载态用，避免页面突然弹出内容。配合 animate-pulse。 */
export function Skeleton({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('animate-pulse rounded-md bg-muted', className)} {...props} />
}

/** 表格行骨架：列数可配，用于列表加载态。 */
export function SkeletonRows({ rows = 5, cols = 4 }: { rows?: number; cols?: number }) {
  return (
    <div className="flex flex-col divide-y divide-border">
      {Array.from({ length: rows }).map((_, r) => (
        <div key={r} className="flex items-center gap-4 px-5 py-4">
          {Array.from({ length: cols }).map((_, c) => (
            <Skeleton key={c} className={cn('h-4', c === 0 ? 'w-40' : 'w-24', c === cols - 1 && 'ml-auto')} />
          ))}
        </div>
      ))}
    </div>
  )
}
