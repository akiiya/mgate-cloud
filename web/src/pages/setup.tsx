import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Cloud, ChevronDown, ChevronRight, ShieldCheck } from 'lucide-react'
import { getSetupStatus, completeSetup } from '@/api/setup'
import { ApiError } from '@/api/client'
import { useAuth } from '@/lib/auth-context'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Callout } from '@/components/ui/callout'
import { ThemeToggle } from '@/components/ui/theme-toggle'

/**
 * 首次初始化页面（无配置启动）。
 * 单页卡片，尽量少填：仅管理员密码必填，其余给出合理默认值。
 */
export function SetupPage() {
  const navigate = useNavigate()
  const { recheck } = useAuth()

  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [mode, setMode] = useState('dev')
  const [baseURL, setBaseURL] = useState('')
  const [httpAddr, setHTTPAddr] = useState(':8080')
  const [dbPath, setDBPath] = useState('./data/mgate-cloud.db')
  const [appSecret, setAppSecret] = useState('')
  const [showAdvanced, setShowAdvanced] = useState(false)

  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  // 拉取默认值预填；base_url 优先用当前浏览器地址（更贴合用户实际访问入口）。
  useEffect(() => {
    let active = true
    getSetupStatus()
      .then((s) => {
        if (!active) return
        setMode(s.defaults.mode || 'dev')
        setHTTPAddr(s.defaults.http_addr || ':8080')
        setDBPath(s.defaults.db_path || './data/mgate-cloud.db')
        setBaseURL(window.location.origin || s.defaults.base_url || '')
      })
      .catch(() => {
        setBaseURL(window.location.origin)
      })
    return () => {
      active = false
    }
  }, [])

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (password.length < 8) {
      setError('管理员密码至少 8 位')
      return
    }
    if (password !== confirm) {
      setError('两次输入的密码不一致')
      return
    }
    setSubmitting(true)
    try {
      const res = await completeSetup({
        admin_username: username.trim() || 'admin',
        admin_password: password,
        admin_password_confirm: confirm,
        mode,
        base_url: baseURL.trim(),
        http_addr: httpAddr.trim(),
        db_path: dbPath.trim(),
        app_secret: appSecret.trim(),
      })
      // 重新探测状态后进入登录页。
      await recheck()
      if (res.restart_recommended) {
        alert('配置已保存。部分设置（运行模式 / app_secret / Cookie Secure）需重启服务后完全生效。\n现在可以先登录。')
      }
      navigate('/login', { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '初始化失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="bg-aurora relative flex min-h-full items-center justify-center px-4 py-12">
      <div className="absolute right-4 top-4">
        <ThemeToggle />
      </div>
      <div className="w-full max-w-md">
        <div className="mb-8 flex flex-col items-center text-center">
          <span className="mb-4 flex h-12 w-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground shadow-soft">
            <Cloud className="h-6 w-6" aria-hidden />
          </span>
          <h1 className="text-2xl font-semibold tracking-tight">初始化 mgate-cloud</h1>
          <p className="mt-1 text-sm text-muted-foreground">首次启动，创建管理员并生成配置文件</p>
        </div>

        <div className="rounded-2xl border border-border bg-card p-6 shadow-soft sm:p-7">
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="su-username">管理员用户名</Label>
              <Input id="su-username" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="admin" />
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="su-pw">管理员密码</Label>
                <Input id="su-pw" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="至少 8 位" required />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="su-pw2">确认密码</Label>
                <Input id="su-pw2" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required />
              </div>
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="su-mode">运行模式</Label>
                <select
                  id="su-mode"
                  value={mode}
                  onChange={(e) => setMode(e.target.value)}
                  className="h-11 rounded-md border border-input bg-card px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <option value="dev">dev（开发）</option>
                  <option value="prod">prod（生产）</option>
                </select>
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="su-baseurl">对外访问地址</Label>
                <Input id="su-baseurl" value={baseURL} onChange={(e) => setBaseURL(e.target.value)} placeholder="https://cloud.example.com" />
              </div>
            </div>

            {/* 高级设置（可选，默认折叠） */}
            <button
              type="button"
              onClick={() => setShowAdvanced((v) => !v)}
              className="flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
            >
              {showAdvanced ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
              高级设置
            </button>
            {showAdvanced && (
              <div className="flex flex-col gap-4 rounded-md border border-border bg-muted/30 p-4">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="su-addr">监听地址</Label>
                  <Input id="su-addr" value={httpAddr} onChange={(e) => setHTTPAddr(e.target.value)} placeholder=":8080" />
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="su-db">数据库路径</Label>
                  <Input id="su-db" value={dbPath} onChange={(e) => setDBPath(e.target.value)} />
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="su-secret">app_secret（设备码签名密钥）</Label>
                  <Input id="su-secret" value={appSecret} onChange={(e) => setAppSecret(e.target.value)} placeholder="留空自动生成" />
                  <p className="text-xs text-muted-foreground">用于设备码签名；生产环境必须固定保存，丢失会影响已发设备码校验。</p>
                </div>
              </div>
            )}

            {error && <Callout tone="error">{error}</Callout>}

            <Button type="submit" loading={submitting} className="mt-1 w-full">
              {submitting ? '初始化中…' : '完成初始化'}
            </Button>
          </form>
        </div>

        <p className="mt-6 flex items-center justify-center gap-1.5 text-xs text-muted-foreground">
          <ShieldCheck className="h-3.5 w-3.5" aria-hidden />
          配置将写入 config.yaml；密码仅保存哈希，不保存明文
        </p>
      </div>
    </div>
  )
}
