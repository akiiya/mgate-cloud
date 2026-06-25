package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// openTestDB 在临时目录创建并迁移一个独立数据库，测试结束自动清理。
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// TestMigrateIdempotent 验证迁移可重复执行而不报错、不重复应用。
func TestMigrateIdempotent(t *testing.T) {
	database := openTestDB(t)

	// 连续执行两次，第二次应为无操作且不报错。
	if err := Migrate(database); err != nil {
		t.Fatalf("首次迁移失败: %v", err)
	}
	if err := Migrate(database); err != nil {
		t.Fatalf("重复迁移应当幂等，但失败: %v", err)
	}

	// 断言关键表均已创建。
	for _, table := range []string{"admins", "admin_sessions", "audit_logs", "schema_migrations"} {
		if !tableExists(t, database, table) {
			t.Errorf("迁移后应存在表 %q", table)
		}
	}

	// 断言迁移记录数等于迁移文件数（未因重复执行而翻倍）。
	files, err := migrationFiles()
	if err != nil {
		t.Fatalf("读取迁移文件失败: %v", err)
	}
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM schema_migrations;").Scan(&count); err != nil {
		t.Fatalf("统计迁移记录失败: %v", err)
	}
	if count != len(files) {
		t.Errorf("迁移记录数 %d 与迁移文件数 %d 不一致", count, len(files))
	}
}

// TestOpenEnablesWAL 验证打开连接后 WAL 已生效。
func TestOpenEnablesWAL(t *testing.T) {
	database := openTestDB(t)

	mode, err := VerifyWAL(database)
	if err != nil {
		t.Fatalf("读取 journal_mode 失败: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode 应为 wal，实际 %q", mode)
	}
}

// tableExists 通过 sqlite_master 判断表是否存在。
func tableExists(t *testing.T, database *sql.DB, name string) bool {
	t.Helper()
	var found string
	err := database.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name=?;", name,
	).Scan(&found)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("查询表是否存在失败: %v", err)
	}
	return found == name
}
