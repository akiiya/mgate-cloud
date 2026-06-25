import { HashRouter } from 'react-router-dom'
import { AuthProvider } from '@/lib/auth-context'
import { AppRoutes } from '@/routes'

/**
 * 应用根组件。
 *
 * 使用 HashRouter（hash 路由）：URL 形如 /#/login、/#/dashboard。
 * 之所以选 hash 模式——服务端只需返回同一个 index.html，无需理解前端路径，
 * 因此页面刷新永远不会 404，对内嵌静态资源的单二进制部署最友好。
 */
export function App() {
  return (
    <HashRouter>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </HashRouter>
  )
}
