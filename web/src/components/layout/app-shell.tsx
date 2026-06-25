import { type ReactNode } from 'react'
import { TopNav } from './top-nav'

/**
 * 应用外壳：为已登录区域提供统一的顶部导航与内容容器。
 * 把布局与页面内容解耦，各页面只关注自身内容。
 */
export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-full">
      <TopNav />
      <main className="mx-auto max-w-6xl px-4 py-8 sm:px-6">{children}</main>
    </div>
  )
}
