// Package webui 负责把内嵌的前端构建产物作为 SPA 对外提供。
package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// indexFile 是 SPA 的入口文件名。
const indexFile = "index.html"

// NewSPAHandler 基于给定的文件系统（通常是内嵌的 web/dist）构造 SPA 处理器。
//
// 行为：
//   - 请求路径命中真实静态文件（如 /assets/app.js）→ 直接返回该文件，Content-Type
//     由标准库按扩展名自动设置。
//   - 路径不存在 → 回退返回 index.html，交由前端 hash 路由处理。
//     因为前端使用 hash 路由（/#/login），真实路径几乎只有 "/"，但该回退能保证
//     任何意外的深链或刷新都不会 404。
//
// 注意：本处理器只挂载在 "/"，而 "/api/" 由更具体的路由前缀优先匹配，
// 因此 API 请求不会被 SPA 回退吞掉。
func NewSPAHandler(dist fs.FS) http.Handler {
	fileServer := http.FileServerFS(dist)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := resolveName(r.URL.Path)

		// 文件存在则交给标准文件服务器（自动处理 Content-Type、缓存校验等）。
		if fileExists(dist, name) {
			fileServer.ServeHTTP(w, r)
			return
		}

		// 否则回退到 SPA 入口，保证刷新/深链不 404。
		serveIndex(w, r, dist)
	})
}

// resolveName 把 URL 路径规范化为相对于 dist 的文件名。
func resolveName(urlPath string) string {
	clean := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
	if clean == "" {
		return indexFile
	}
	return clean
}

// fileExists 判断 dist 中是否存在指定的常规文件。
func fileExists(dist fs.FS, name string) bool {
	f, err := dist.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		return false
	}
	return true
}

// serveIndex 返回 index.html 作为 SPA 回退入口。
func serveIndex(w http.ResponseWriter, r *http.Request, dist fs.FS) {
	data, err := fs.ReadFile(dist, indexFile)
	if err != nil {
		// 仅在前端从未构建（仅占位文件缺失）时发生；给出清晰提示而非裸 500。
		http.Error(w, "前端资源未就绪：请先执行 npm --prefix web run build", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
