import { type ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { useAuth } from '@/lib/auth-context'
import { AppShell } from '@/components/layout/app-shell'
import { LoginPage } from '@/pages/login'
import { DashboardPage } from '@/pages/dashboard'
import { DevicesPage } from '@/pages/devices'
import { DeviceDetailPage } from '@/pages/device-detail'
import { CommandsPage } from '@/pages/commands'
import { CommandDetailPage } from '@/pages/command-detail'
import { NotFoundPage } from '@/pages/not-found'

/** 会话探测期间的全屏加载，避免登录态未确定时的页面闪烁。 */
function FullScreenLoader() {
  return (
    <div className="flex min-h-full items-center justify-center">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" aria-hidden />
    </div>
  )
}

/**
 * 受保护路由：仅允许已登录用户访问，未登录重定向到 /login。
 * 通过 AppShell 包裹，保证受保护页面拥有统一的导航布局。
 */
function ProtectedRoute({ children }: { children: ReactNode }) {
  const { user, initializing } = useAuth()

  if (initializing) return <FullScreenLoader />
  if (!user) return <Navigate to="/login" replace />
  return <AppShell>{children}</AppShell>
}

/**
 * 应用路由表（hash 模式由上层 HashRouter 提供）。
 * 路径示例：/#/login、/#/dashboard。
 */
export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/dashboard"
        element={
          <ProtectedRoute>
            <DashboardPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/devices"
        element={
          <ProtectedRoute>
            <DevicesPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/devices/:deviceId"
        element={
          <ProtectedRoute>
            <DeviceDetailPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/commands"
        element={
          <ProtectedRoute>
            <CommandsPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/commands/:commandId"
        element={
          <ProtectedRoute>
            <CommandDetailPage />
          </ProtectedRoute>
        }
      />
      {/* 根路径重定向到 dashboard；未登录会被 ProtectedRoute 再导向 login */}
      <Route path="/" element={<Navigate to="/dashboard" replace />} />
      {/* 兜底 404 */}
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}
