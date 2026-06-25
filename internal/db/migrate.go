package db

import (
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"mgate-cloud/migrations"
)

// Migrate 执行所有尚未应用的迁移脚本，幂等可重复调用。
//
// 策略：以 schema_migrations 表记录已应用的版本（版本号即文件名），
// 启动时把内嵌的 .sql 按文件名排序后逐个比对，未应用者在事务中执行并登记。
// 这样多次启动、或在已有数据库上启动都安全无副作用。
func Migrate(database *sql.DB) error {
	if err := ensureMigrationsTable(database); err != nil {
		return err
	}

	applied, err := appliedVersions(database)
	if err != nil {
		return err
	}

	files, err := migrationFiles()
	if err != nil {
		return err
	}

	for _, name := range files {
		if applied[name] {
			continue // 已应用，跳过——这是幂等性的核心
		}
		if err := applyMigration(database, name); err != nil {
			return err
		}
	}
	return nil
}

// ensureMigrationsTable 创建版本记录表（若不存在）。
func ensureMigrationsTable(database *sql.DB) error {
	const ddl = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at DATETIME NOT NULL
		);`
	if _, err := database.Exec(ddl); err != nil {
		return fmt.Errorf("db: 创建 schema_migrations 失败: %w", err)
	}
	return nil
}

// appliedVersions 读取已应用版本集合。
func appliedVersions(database *sql.DB) (map[string]bool, error) {
	rows, err := database.Query("SELECT version FROM schema_migrations;")
	if err != nil {
		return nil, fmt.Errorf("db: 读取已应用迁移失败: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("db: 扫描迁移版本失败: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// migrationFiles 返回内嵌迁移脚本的文件名，按字典序排序。
//
// 文件名遵循 NNN_name.sql 约定，字典序即为执行顺序。
func migrationFiles() ([]string, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("db: 读取内嵌迁移目录失败: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// applyMigration 在单个事务中执行一份迁移脚本并登记版本。
//
// 把"执行脚本"与"写入版本记录"放进同一事务，保证二者原子：
// 要么都成功，要么都不留痕，杜绝"脚本执行了但版本没记上"的半成品状态。
func applyMigration(database *sql.DB, name string) error {
	content, err := migrations.FS.ReadFile(name)
	if err != nil {
		return fmt.Errorf("db: 读取迁移 %q 失败: %w", name, err)
	}

	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("db: 为迁移 %q 开启事务失败: %w", name, err)
	}

	if _, err := tx.Exec(string(content)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("db: 执行迁移 %q 失败: %w", name, err)
	}

	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?);",
		name, time.Now().UTC(),
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("db: 登记迁移 %q 失败: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: 提交迁移 %q 失败: %w", name, err)
	}
	return nil
}
