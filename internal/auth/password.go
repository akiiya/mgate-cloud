// Package auth 实现管理员认证：口令哈希、会话、CSRF 与登录/登出流程。
package auth

import (
	"fmt"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost 是 bcrypt 的计算代价因子。
//
// 选用略高于库默认（10）的 12：在现代服务器上单次哈希约几十毫秒，
// 既能显著抬高离线爆破成本，又不至于拖慢登录响应。
const bcryptCost = 12

// dummyHash 是一个固定口令的 bcrypt 哈希，仅用于计时对齐（见 DummyVerify）。
// 用 sync.Once 惰性初始化，避免在进程启动时引入一次 bcrypt 计算延迟。
var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

// DummyVerify 执行一次与 VerifyPassword 等价耗时的 bcrypt 比较（结果必然不匹配且被丢弃）。
//
// 用途：在"用户名不存在 / 账户被禁用"等不会真正比对口令的分支调用它，
// 使这些分支与"用户存在且比对口令"分支的响应耗时相近，
// 消除可被用于枚举有效用户名的计时侧信道。
func DummyVerify(password string) {
	dummyHashOnce.Do(func() {
		if h, err := bcrypt.GenerateFromPassword([]byte("mgate-cloud-timing-equalizer"), bcryptCost); err == nil {
			dummyHash = h
		}
	})
	if dummyHash != nil {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
	}
}

// HashPassword 对明文口令做 bcrypt 哈希。
//
// bcrypt 自带随机盐并将盐与代价编码进结果字符串，因此无需额外管理盐。
func HashPassword(plain string) (string, error) {
	if plain == "" {
		return "", fmt.Errorf("auth: 口令不能为空")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("auth: 口令哈希失败: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword 校验明文口令与哈希是否匹配。
//
// bcrypt.CompareHashAndPassword 内部为恒定时间比较，可抵御计时侧信道。
// 返回 bool 而非 error，让调用方专注于"是否匹配"的业务语义。
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
