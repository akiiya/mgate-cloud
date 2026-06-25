import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { getMe, login as loginApi, logout as logoutApi, type AdminUser } from '@/api/auth'
import { getSetupStatus } from '@/api/setup'

/**
 * 认证上下文。
 *
 * 仅维护"当前用户 + setup 状态 + 加载态"这一最小必要状态，刻意不引入重型状态管理库。
 */
interface AuthContextValue {
  user: AdminUser | null
  /** 系统是否尚未初始化（需进入 Setup）。 */
  setupRequired: boolean
  /** 初次探测是否仍在进行，用于避免未确定状态时闪烁跳转。 */
  initializing: boolean
  signIn: (username: string, password: string) => Promise<void>
  signOut: () => Promise<void>
  /** 重新探测 setup/会话状态（如 Setup 完成后调用）。 */
  recheck: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AdminUser | null>(null)
  const [setupRequired, setSetupRequired] = useState(false)
  const [initializing, setInitializing] = useState(true)

  // 探测流程：先看是否需要 setup；需要则不再查会话，引导到 Setup 页。
  const probe = useCallback(async () => {
    try {
      const status = await getSetupStatus()
      if (status.setup_required) {
        setSetupRequired(true)
        setUser(null)
        return
      }
      setSetupRequired(false)
    } catch {
      // setup 状态查询失败时按"不需要 setup"处理，继续走会话探测。
      setSetupRequired(false)
    }
    try {
      setUser(await getMe())
    } catch {
      setUser(null)
    }
  }, [])

  useEffect(() => {
    let active = true
    probe().finally(() => {
      if (active) setInitializing(false)
    })
    return () => {
      active = false
    }
  }, [probe])

  const signIn = useCallback(async (username: string, password: string) => {
    setUser(await loginApi(username, password))
  }, [])

  const signOut = useCallback(async () => {
    await logoutApi()
    setUser(null)
  }, [])

  const recheck = useCallback(async () => {
    await probe()
  }, [probe])

  return (
    <AuthContext.Provider value={{ user, setupRequired, initializing, signIn, signOut, recheck }}>
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
