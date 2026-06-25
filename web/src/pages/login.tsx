import { useState, type FormEvent } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import { Cloud, ShieldCheck } from 'lucide-react'
import { useAuth } from '@/lib/auth-context'
import { ApiError } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ErrorAlert } from '@/components/ui/alert'

/**
 * 登录页。
 * - 居中卡片布局，现代浅色风格。
 * - 表单提交（含回车）触发登录，期间展示 loading 并禁用按钮。
 * - 登录失败展示后端返回的统一错误信息。
 * - 已登录用户直接重定向到 dashboard，避免重复登录。
 */
export function LoginPage() {
  const { user, initializing, signIn } = useAuth()
  const navigate = useNavigate()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  // 会话探测完成且已登录：跳转到 dashboard。
  if (!initializing && user) {
    return <Navigate to="/dashboard" replace />
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await signIn(username.trim(), password)
      navigate('/dashboard', { replace: true })
    } catch (err) {
      // 统一错误提示：优先使用后端可读 message。
      setError(err instanceof ApiError ? err.message : '登录失败，请稍后重试')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="bg-grid flex min-h-full items-center justify-center px-4 py-12">
      <div className="w-full max-w-sm">
        {/* 品牌标识 */}
        <div className="mb-8 flex flex-col items-center text-center">
          <span className="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-soft">
            <Cloud className="h-6 w-6" aria-hidden />
          </span>
          <h1 className="text-2xl font-semibold tracking-tight">mgate-cloud</h1>
          <p className="mt-1 text-sm text-muted-foreground">安全管理你的 mgate 设备</p>
        </div>

        <div className="rounded-lg border border-border bg-card p-6 shadow-soft sm:p-7">
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="username">用户名</Label>
              <Input
                id="username"
                autoComplete="username"
                autoFocus
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="请输入管理员用户名"
                required
              />
            </div>

            <div className="flex flex-col gap-1.5">
              <Label htmlFor="password">密码</Label>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="请输入密码"
                required
              />
            </div>

            {error && <ErrorAlert message={error} />}

            <Button type="submit" loading={submitting} className="mt-1 w-full">
              {submitting ? '登录中…' : '登录'}
            </Button>
          </form>
        </div>

        <p className="mt-6 flex items-center justify-center gap-1.5 text-xs text-muted-foreground">
          <ShieldCheck className="h-3.5 w-3.5" aria-hidden />
          会话采用 HttpOnly Cookie，并启用 CSRF 防护
        </p>
      </div>
    </div>
  )
}
