package util

import "time"

// Clock 抽象"当前时间"的来源。
//
// 引入该接口是为了让依赖时间的逻辑（如会话过期判断）可在测试中注入固定时间，
// 而非散落地直接调用 time.Now()，从而获得确定性测试。
type Clock interface {
	Now() time.Time
}

// SystemClock 是基于真实系统时钟的默认实现。
type SystemClock struct{}

// Now 返回当前 UTC 时间。
//
// 统一使用 UTC 入库，避免不同部署时区导致的时间歧义；展示层再做本地化。
func (SystemClock) Now() time.Time { return time.Now().UTC() }
