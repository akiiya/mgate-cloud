import { NavLink } from 'react-router-dom'
import { Cloud } from 'lucide-react'
import { navItems } from '@/lib/nav'
import { cn } from '@/lib/cn'

/**
 * 侧栏内容：品牌标识 + 主导航 + 底部安全声明。
 * 桌面常驻、移动端在抽屉中复用同一组件，onNavigate 用于移动端点击后关闭抽屉。
 */
export function SidebarContent({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <div className="flex h-full flex-col">
      {/* 品牌 */}
      <div className="flex h-16 items-center gap-2.5 px-5">
        <span className="flex h-9 w-9 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-soft">
          <Cloud className="h-5 w-5" aria-hidden />
        </span>
        <div className="leading-tight">
          <p className="text-sm font-semibold">mgate-cloud</p>
          <p className="text-xs text-muted-foreground">设备控制面</p>
        </div>
      </div>

      {/* 主导航 */}
      <nav className="flex-1 space-y-1 px-3 py-4">
        {navItems.map((item) => {
          const Icon = item.icon
          return (
            <NavLink
              key={item.to}
              to={item.to}
              onClick={onNavigate}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-primary/10 text-primary'
                    : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                )
              }
            >
              <Icon className="h-[18px] w-[18px] shrink-0" aria-hidden />
              {item.label}
            </NavLink>
          )
        })}
      </nav>

      {/* 底部安全声明：持续提醒产品边界。 */}
      <div className="border-t border-border px-5 py-4">
        <p className="text-xs leading-relaxed text-muted-foreground">
          控制面仅经 agent 下发白名单操作，不执行远程 shell。
        </p>
      </div>
    </div>
  )
}
