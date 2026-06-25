// Package migrations 通过 go:embed 内嵌所有 SQL 迁移脚本。
//
// 之所以把 embed 放在 migrations 目录自身（而非 internal/db），是因为
// go:embed 不允许引用上级目录（不能写 ../../migrations）。让迁移脚本与
// 其 embed 声明同处一目录，是符合 Go 约束的最简洁做法。
package migrations

import "embed"

// FS 内嵌全部 .sql 迁移文件。迁移器按文件名字典序依次执行。
//
//go:embed *.sql
var FS embed.FS
