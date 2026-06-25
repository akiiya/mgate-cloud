import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { getMe, login as loginApi, logout as logoutApi, type AdminUser } from '@/api/auth'

/**
 * 认证上下文。
 *
 * 仅维护"当前用户 + 加载态"这一最小必要状态，刻意不引入重型状态管理库——
 * Phase 1 的认证需求简单，useState + Context 足矣，符合"避免过度工程"的要求。
 */
interface AuthContextValue {
  user: AdminUser | null
  /** 初次会话探测是否仍在进行，用于避免未确定登录态时闪烁跳转。 */
  initializing: boolean
  signIn: (username: string, password: string) => Promise<void>
  signOut: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AdminUser | null>(null)
  const [initializing, setInitializing] = useState(true)

  // 应用启动时探测一次会话：已登录则填充用户，未登录（401）则保持 null。
  useEffect(() => {
    let active = true
    getMe()
      .then((u) => {
        if (active) setUser(u)
      })
      .catch(() => {
        if (active) setUser(null)
      })
      .finally(() => {
        if (active) setInitializing(false)
      })
    return () => {
      active = false
    }
  }, [])

  const signIn = useCallback(async (username: string, password: string) => {
    const u = await loginApi(username, password)
    setUser(u)
  }, [])

  const signOut = useCallback(async () => {
    await logoutApi()
    setUser(null)
  }, [])

  return (
    <AuthContext.Provider value={{ user, initializing, signIn, signOut }}>
      {children}
    </AuthContext.Provider>
  )
}

/** useAuth 读取认证上下文；脱离 AuthProvider 使用会抛错以尽早暴露集成问题。 */
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth 必须在 AuthProvider 内使用')
  }
  return ctx
}
