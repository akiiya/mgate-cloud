package util

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID 生成一个用于数据库主键的随机标识符。
//
// 使用 crypto/rand 而非时间戳或自增，保证不可预测、无碰撞顾虑，
// 同时不暴露记录的创建顺序。16 字节（128 bit）熵足以避免实际碰撞。
func NewID() string {
	var b [16]byte
	// crypto/rand.Read 在现代运行时不会失败；若失败属于不可恢复的系统级错误。
	if _, err := rand.Read(b[:]); err != nil {
		panic("util: 无法从系统获取随机数: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
