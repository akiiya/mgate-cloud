package config

import (
	"os"
	"strconv"
	"strings"
)

// 本文件集中放置环境变量读取的小工具，统一默认值处理逻辑，
// 避免在 Load 中重复编写"取值-判空-回退默认"的样板代码。

// envString 读取字符串环境变量，为空时返回默认值。
func envString(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// envBool 读取布尔环境变量，支持 true/false/1/0 等常见写法，无法解析时回退默认值。
func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return parsed
}

// pickStr 以"环境变量 > 文件值 > 默认值"返回字符串。
func pickStr(envKey, fileVal, def string) string {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v
	}
	if fileVal != "" {
		return fileVal
	}
	return def
}

// pickBool 以"环境变量 > 文件值 > 默认值"返回布尔。fileVal 为 nil 表示文件未设置。
func pickBool(envKey string, fileVal *bool, def bool) bool {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
		return def
	}
	if fileVal != nil {
		return *fileVal
	}
	return def
}

// envInt 读取整数环境变量，无法解析时回退默认值。
func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return parsed
}
