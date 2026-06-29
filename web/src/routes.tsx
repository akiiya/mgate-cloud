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
import { UpdatePage } from '@/pages/update'
import { SettingsPage } from '@/pages/settings'
import { SetupPage } from '@/pages/setup'
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
  const { initializing, setupRequired } = useAuth()

  // 探测期间统一显示加载，避免在 setup/login 之间闪烁。
  if (initializing) return <FullScreenLoader />

  // 未初始化：强制进入 Setup，其余路径一律重定向到 /setup。
  if (setupRequired) {
    return (
      <Routes>
        <Route path="/setup" element={<SetupPage />} />
        <Route path="*" element={<Navigate to="/setup" replace />} />
      </Routes>
    )
  }

  return (
    <Routes>
      {/* 已初始化时访问 /setup 直接回首页 */}
      <Route path="/setup" element={<Navigate to="/dashboard" replace />} />
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
      <Route
        path="/update"
        element={
          <ProtectedRoute>
            <UpdatePage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/settings"
        element={
          <ProtectedRoute>
            <SettingsPage />
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
