import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath, URL } from 'node:url'

// Vite 配置。
// - 路径别名 @ 对应 src，与 tsconfig 保持一致。
// - 开发模式下把 /api 代理到本地 Go 后端（默认 :8080），
//   使前端 dev server 与后端 API 同源，无需处理跨域与 cookie 问题。
// - 生产构建输出到 dist，供 Go embed 内嵌。
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
