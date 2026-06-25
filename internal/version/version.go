// Package version 暴露构建版本信息。
//
// Version 默认为 "dev"，发布构建通过 ldflags 注入真实版本，例如：
//
//	go build -ldflags "-X mgate-cloud/internal/version.Version=$(cat VERSION)" ...
//
// 这样最终二进制能在启动日志中报告自身版本，便于运维核对。
package version

// Version 是当前构建的版本号；由 ldflags 注入，缺省为 dev。
var Version = "dev"
