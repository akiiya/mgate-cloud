// Package db 负责 SQLite 连接的建立、PRAGMA 配置与连接池管理。
//
// 数据库访问在此集中管理：上层通过 *sql.DB 操作，PRAGMA 等底层细节不外泄。
package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	// 使用纯 Go 实现的 SQLite 驱动（modernc.org/sqlite），无需 CGO。
	// 这是单二进制、跨平台（尤其 Windows 无 gcc）构建的关键：
	// 编译产物自包含，不依赖系统 libsqlite3。
	_ "modernc.org/sqlite"
)

// driverName 是 modernc.org/sqlite 注册的驱动名。
const driverName = "sqlite"

// Open 打开（必要时创建）SQLite 数据库，应用统一的 PRAGMA 并返回连接池。
//
// PRAGMA 通过 DSN 的 _pragma 参数下发，确保连接池中"每一条连接"建立时都自动套用，
// 而不是只在某一条连接上执行一次——这是 database/sql 连接池下的正确做法。
func Open(dbPath string) (*sql.DB, error) {
	if err := ensureDir(dbPath); err != nil {
		return nil, err
	}

	dsn := buildDSN(dbPath)
	database, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("db: 打开数据库失败: %w", err)
	}

	// 立即 Ping 一次，把"配置错误/无法创建文件"等问题在启动期暴露，而非首次查询时。
	if err := database.Ping(); err != nil {
		database.Close()
		return nil, fmt.Errorf("db: 连接数据库失败: %w", err)
	}

	return database, nil
}

// buildDSN 构造带 PRAGMA 的 modernc.org/sqlite DSN。
//
// 各 PRAGMA 含义：
//   - journal_mode=WAL：开启 WAL 日志，读写并发更友好（读不阻塞写）。
//   - foreign_keys=ON：SQLite 默认关闭外键约束，必须显式开启。
//   - busy_timeout=5000：遇到锁时最多等待 5s，缓解偶发 "database is locked"。
//   - synchronous=NORMAL：WAL 下兼顾安全与性能的常用档位。
//
// 此外通过 _txlock=immediate 让所有显式事务以 BEGIN IMMEDIATE 开启：事务一开始
// 即获取写锁，从而把并发写事务严格串行化。这能避免 SQLite "先读后写"时由
// SHARED→RESERVED 升级引发的死锁式 SQLITE_BUSY，使 enroll 这类"读校验后写入"
// 的并发场景表现确定（败者干净地看到"已使用"，而非随机的 database is locked）。
func buildDSN(dbPath string) string {
	pragmas := []string{
		"journal_mode(WAL)",
		"foreign_keys(ON)",
		"busy_timeout(5000)",
		"synchronous(NORMAL)",
	}

	q := url.Values{}
	for _, p := range pragmas {
		q.Add("_pragma", p)
	}
	q.Set("_txlock", "immediate")
	// 形如：file:./data/mgate-cloud.db?_pragma=journal_mode(WAL)&_pragma=...&_txlock=immediate
	return "file:" + dbPath + "?" + q.Encode()
}

// ensureDir 确保数据库文件所在目录存在，不存在则创建。
//
// 启动期保证目录就绪，避免首次写入因目录缺失而失败。
func ensureDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("db: 创建数据目录 %q 失败: %w", dir, err)
	}
	return nil
}

// VerifyWAL 查询当前 journal_mode，确认 WAL 已生效。
//
// 用于启动自检与测试断言；返回实际的 journal_mode 字符串。
func VerifyWAL(database *sql.DB) (string, error) {
	var mode string
	if err := database.QueryRow("PRAGMA journal_mode;").Scan(&mode); err != nil {
		return "", fmt.Errorf("db: 读取 journal_mode 失败: %w", err)
	}
	return mode, nil
}
