import { useState } from 'react'
import { NavLink } from 'react-router-dom'
import { Cloud, LayoutDashboard, LogOut, Router, Terminal, UserRound } from 'lucide-react'
import { useAuth } from '@/lib/auth-context'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/cn'

/** 顶部导航项定义，数据驱动渲染。 */
const navItems = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/devices', label: '设备管理', icon: Router },
  { to: '/commands', label: '命令记录', icon: Terminal },
]

/**
 * 顶部导航栏：产品标识 + 主导航 + 当前管理员 + 登出。
 * 登出由 AuthProvider 统一处理，登出后路由守卫会把用户带回登录页。
 */
export function TopNav() {
  const { user, signOut } = useAuth()
  const [signingOut, setSigningOut] = useState(false)

  async function handleSignOut() {
    setSigningOut(true)
    try {
      await signOut()
    } finally {
      setSigningOut(false)
    }
  }

  return (
    <header className="sticky top-0 z-10 border-b border-border bg-card/80 backdrop-blur">
      <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-4 sm:px-6">
        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2.5">
            <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-soft">
              <Cloud className="h-5 w-5" aria-hidden />
            </span>
            <p className="hidden text-sm font-semibold sm:block">mgate-cloud</p>
          </div>

          <nav className="flex items-center gap-1">
            {navItems.map((item) => {
              const Icon = item.icon
              return (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) =>
                    cn(
                      'flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                      isActive ? 'bg-muted text-foreground' : 'text-muted-foreground hover:bg-muted/60',
                    )
                  }
                >
                  <Icon className="h-4 w-4" aria-hidden />
                  {item.label}
                </NavLink>
              )
            })}
          </nav>
        </div>

        <div className="flex items-center gap-3">
          <span className="hidden items-center gap-1.5 text-sm text-muted-foreground sm:flex">
            <UserRound className="h-4 w-4" aria-hidden />
            {user?.username}
          </span>
          <Button variant="outline" size="sm" loading={signingOut} onClick={handleSignOut}>
            <LogOut className="h-4 w-4" aria-hidden />
            登出
          </Button>
        </div>
      </div>
    </header>
  )
}
