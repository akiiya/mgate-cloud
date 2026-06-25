// Package web 通过 go:embed 内嵌前端构建产物（web/dist）。
//
// 为何 embed 声明放在 web 目录而非 internal/webui：go:embed 不允许引用上级目录
// （不能写 ../../web/dist），因此承载 embed 的 .go 文件必须与 dist 同处一处。
// internal/webui 则消费这里导出的 fs.FS，负责静态资源的 HTTP 服务。
package web

import (
	"embed"
	"io/fs"
)

// distFS 内嵌整个 dist 目录。
//
// 使用 all: 前缀以包含以 "." / "_" 开头的文件（某些构建工具会产出此类资源）。
// 注意：编译期 dist 目录必须存在且至少含一个文件，否则 go build 失败。
// 仓库中提供了占位的 dist/index.html 作为兜底，真实构建（npm run build）会覆盖它。
//
//go:embed all:dist
var distFS embed.FS

// DistFS 返回以 dist 为根的子文件系统，使调用方可用 "index.html" 这样的相对路径访问。
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
