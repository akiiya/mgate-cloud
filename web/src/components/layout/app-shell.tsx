import { useEffect, useState, type ReactNode } from 'react'
import { useLocation } from 'react-router-dom'
import { X } from 'lucide-react'
import { SidebarContent } from './sidebar'
import { Topbar } from './topbar'

/**
 * 应用外壳：桌面端常驻左侧栏 + 顶栏 + 全宽内容区；移动端侧栏改为抽屉。
 * 把布局与页面解耦，各页只关注自身内容。
 */
export function AppShell({ children }: { children: ReactNode }) {
  const [drawerOpen, setDrawerOpen] = useState(false)
  const location = useLocation()

  // 路由变化时自动收起移动端抽屉。
  useEffect(() => {
    setDrawerOpen(false)
  }, [location.pathname])

  return (
    <div className="flex min-h-full">
      {/* 桌面常驻侧栏 */}
      <aside className="hidden w-64 shrink-0 border-r border-border bg-sidebar text-sidebar-foreground lg:block">
        <div className="sticky top-0 h-screen">
          <SidebarContent />
        </div>
      </aside>

      {/* 移动端抽屉 */}
      {drawerOpen && (
        <div className="fixed inset-0 z-40 lg:hidden">
          <div className="absolute inset-0 bg-foreground/40 backdrop-blur-sm" onClick={() => setDrawerOpen(false)} aria-hidden />
          <aside className="absolute left-0 top-0 h-full w-64 animate-slide-in border-r border-border bg-sidebar text-sidebar-foreground shadow-xl">
            <button
              type="button"
              onClick={() => setDrawerOpen(false)}
              aria-label="关闭导航"
              className="absolute right-3 top-4 flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground hover:bg-muted"
            >
              <X className="h-5 w-5" aria-hidden />
            </button>
            <SidebarContent onNavigate={() => setDrawerOpen(false)} />
          </aside>
        </div>
      )}

      {/* 主区：顶栏 + 全宽内容 */}
      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar onOpenSidebar={() => setDrawerOpen(true)} />
        <main className="flex-1 px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
          <div className="mx-auto w-full max-w-7xl animate-fade-in">{children}</div>
        </main>
      </div>
    </div>
  )
}
