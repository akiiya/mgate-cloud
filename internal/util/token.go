package util

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// RandomToken 生成一个 URL 安全的高熵随机令牌，熵来自 crypto/rand。
//
// nBytes 控制随机字节数（即熵）。设备令牌、pairing 令牌都经此生成，
// 通过参数化字节数满足"令牌强度可配置但不可弱化"的要求。
func RandomToken(nBytes int) (string, error) {
	if nBytes <= 0 {
		return "", fmt.Errorf("util: 令牌字节数必须为正，得到 %d", nBytes)
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("util: 生成随机令牌失败: %w", err)
	}
	// RawURLEncoding：无填充、URL/JSON 安全。
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashTokenHex 计算令牌的 SHA-256 十六进制摘要。
//
// 令牌入库前一律走此函数：数据库只见摘要，原始令牌仅一次性下发给持有方。
// 令牌本身已是高熵随机串，无需加盐即可安全比对（不同于低熵口令）。
func HashTokenHex(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ConstantTimeEqualString 以恒定时间比较两个字符串，避免计时侧信道。
//
// 用于一切"秘密相关"的比较（如签名校验），杜绝按字符提前返回带来的时序泄露。
func ConstantTimeEqualString(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
