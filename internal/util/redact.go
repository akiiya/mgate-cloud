package util

import "strings"

// sensitiveKeys 列出在审计 metadata / 日志中必须脱敏的字段名（小写匹配）。
//
// 安全边界：口令、各类令牌、cookie、鉴权头一律不得明文落库或落日志。
var sensitiveKeys = []string{
	"password",
	"passwd",
	"pwd",
	"token",
	"session",
	"csrf",
	"cookie",
	"authorization",
	"secret",
	"password_hash",
}

// Redact 返回 m 的副本，将其中疑似敏感字段的值替换为占位符。
//
// 采用"按 key 名匹配"的保守策略：只要 key 包含敏感词即脱敏，宁可多脱也不漏脱。
// 该函数用于审计 metadata 写库前的最后一道防线。
func Redact(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if isSensitiveKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = v
	}
	return out
}

// isSensitiveKey 判断字段名是否命中敏感词（大小写不敏感、子串匹配）。
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
