import { useEffect, useRef, useState } from 'react'
import { ChevronDown, LogOut, Menu, UserRound } from 'lucide-react'
import { useAuth } from '@/lib/auth-context'
import { ThemeToggle } from '@/components/ui/theme-toggle'
import { cn } from '@/lib/cn'

/**
 * 顶栏：移动端汉堡菜单 + 主题切换 + 当前管理员菜单（含登出）。
 * 页面标题与操作由各页自身的页头负责，顶栏保持精简。
 */
export function Topbar({ onOpenSidebar }: { onOpenSidebar: () => void }) {
  return (
    <header className="sticky top-0 z-20 flex h-16 items-center gap-3 border-b border-border bg-background/80 px-4 backdrop-blur sm:px-6">
      <button
        type="button"
        onClick={onOpenSidebar}
        aria-label="打开导航"
        className="flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground lg:hidden"
      >
        <Menu className="h-5 w-5" aria-hidden />
      </button>

      <div className="flex-1" />

      <ThemeToggle />
      <UserMenu />
    </header>
  )
}

/** 管理员菜单：显示用户名，展开后可登出。 */
function UserMenu() {
  const { user, signOut } = useAuth()
  const [open, setOpen] = useState(false)
  const [signingOut, setSigningOut] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  // 点击外部关闭。
  useEffect(() => {
    if (!open) return
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [open])

  async function handleSignOut() {
    setSigningOut(true)
    try {
      await signOut()
    } finally {
      setSigningOut(false)
    }
  }

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm hover:bg-muted"
      >
        <span className="flex h-7 w-7 items-center justify-center rounded-full bg-muted text-muted-foreground">
          <UserRound className="h-4 w-4" aria-hidden />
        </span>
        <span className="hidden max-w-[10rem] truncate font-medium sm:block">{user?.username}</span>
        <ChevronDown className={cn('h-4 w-4 text-muted-foreground transition-transform', open && 'rotate-180')} aria-hidden />
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-2 w-48 animate-fade-in rounded-lg border border-border bg-card p-1 shadow-soft">
          <div className="px-3 py-2">
            <p className="text-xs text-muted-foreground">已登录</p>
            <p className="truncate text-sm font-medium">{user?.username}</p>
          </div>
          <div className="my-1 h-px bg-border" />
          <button
            type="button"
            onClick={handleSignOut}
            disabled={signingOut}
            className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-foreground hover:bg-muted disabled:opacity-60"
          >
            <LogOut className="h-4 w-4" aria-hidden />
            {signingOut ? '登出中…' : '登出'}
          </button>
        </div>
      )}
    </div>
  )
}
