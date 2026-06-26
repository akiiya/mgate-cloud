import { LayoutDashboard, Router, Terminal, DownloadCloud, Settings, type LucideIcon } from 'lucide-react'

export interface NavItem {
  to: string
  label: string
  icon: LucideIcon
}

/** 左侧主导航。数据驱动渲染，侧栏与移动抽屉共用同一份定义。 */
export const navItems: NavItem[] = [
  { to: '/dashboard', label: '概览', icon: LayoutDashboard },
  { to: '/devices', label: '设备', icon: Router },
  { to: '/commands', label: '命令记录', icon: Terminal },
  { to: '/update', label: '更新', icon: DownloadCloud },
  { to: '/settings', label: '设置', icon: Settings },
]
