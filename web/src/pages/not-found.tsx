import { useNavigate } from 'react-router-dom'
import { Compass } from 'lucide-react'
import { Button } from '@/components/ui/button'

/**
 * 404 / 兜底页。
 * hash 路由匹配不到时展示，并提供返回 Dashboard 的入口。
 */
export function NotFoundPage() {
  const navigate = useNavigate()

  return (
    <div className="bg-grid flex min-h-full flex-col items-center justify-center px-4 py-16 text-center">
      <span className="mb-5 flex h-12 w-12 items-center justify-center rounded-xl bg-muted text-muted-foreground">
        <Compass className="h-6 w-6" aria-hidden />
      </span>
      <h1 className="text-3xl font-semibold tracking-tight">页面未找到</h1>
      <p className="mt-2 max-w-sm text-sm text-muted-foreground">
        你访问的页面不存在，或链接已失效。
      </p>
      <Button className="mt-6" onClick={() => navigate('/dashboard', { replace: true })}>
        返回 Dashboard
      </Button>
    </div>
  )
}
