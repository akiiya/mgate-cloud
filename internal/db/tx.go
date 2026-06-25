package db

import (
	"context"
	"database/sql"
	"fmt"
)

// WithTx 在事务中执行 fn，并统一处理提交与回滚。
//
// 设计意图：把"开启事务、出错回滚、成功提交、panic 也回滚"这套易错的样板
// 收敛到一处，调用方只需关注业务逻辑，避免每个写操作各写一遍而漏掉回滚。
func WithTx(ctx context.Context, database *sql.DB, fn func(tx *sql.Tx) error) (err error) {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: 开启事务失败: %w", err)
	}

	// 使用 defer + recover 兜底：即便 fn panic，也保证事务被回滚而非悬挂。
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // 回滚后继续向上抛出 panic，不吞掉错误
		}
	}()

	if err = fn(tx); err != nil {
		// 业务出错：回滚。回滚自身的错误仅在原始错误为空时才需关注，此处原始错误优先。
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("db: 事务回滚失败: %v (原始错误: %w)", rbErr, err)
		}
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("db: 事务提交失败: %w", err)
	}
	return nil
}
