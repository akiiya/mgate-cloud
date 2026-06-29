import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'

/** 主题偏好：跟随系统 / 浅色 / 深色。 */
export type Theme = 'system' | 'light' | 'dark'
/** 实际生效的外观。 */
export type Resolved = 'light' | 'dark'

const STORAGE_KEY = 'mgate-theme'

interface ThemeContextValue {
  theme: Theme
  resolved: Resolved
  setTheme: (t: Theme) => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

function systemPrefersDark(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches
}

function resolve(theme: Theme): Resolved {
  if (theme === 'system') return systemPrefersDark() ? 'dark' : 'light'
  return theme
}

/** 把生效外观写到 <html>（class + color-scheme），与 index.html 的预设脚本一致。 */
function apply(resolved: Resolved) {
  const el = document.documentElement
  el.classList.toggle('dark', resolved === 'dark')
  el.style.colorScheme = resolved
}

function readStored(): Theme {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    /* ignore */
  }
  return 'system'
}

/**
 * ThemeProvider：管理 system/light/dark 偏好。
 * - 默认 system，跟随操作系统。
 * - 用户切换后持久化到 localStorage，下次沿用。
 * - system 模式下监听系统外观变化实时切换。
 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(readStored)
  const [resolved, setResolved] = useState<Resolved>(() => resolve(readStored()))

  useEffect(() => {
    const r = resolve(theme)
    setResolved(r)
    apply(r)
  }, [theme])

  // system 模式下跟随系统外观变化。
  useEffect(() => {
    if (theme !== 'system') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => {
      const r: Resolved = mq.matches ? 'dark' : 'light'
      setResolved(r)
      apply(r)
    }
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [theme])

  const setTheme = useCallback((t: Theme) => {
    try {
      localStorage.setItem(STORAGE_KEY, t)
    } catch {
      /* ignore */
    }
    setThemeState(t)
  }, [])

  return <ThemeContext.Provider value={{ theme, resolved, setTheme }}>{children}</ThemeContext.Provider>
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme 必须在 ThemeProvider 内使用')
  return ctx
}
