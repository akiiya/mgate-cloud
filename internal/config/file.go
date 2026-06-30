package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// 配置文件采用扁平 YAML（仅 key: value 标量），故自带极简解析/生成，避免引入 YAML 依赖。
// 这满足本项目"最小依赖"的取向；schema 是受控的扁平结构，无需完整 YAML。

const defaultConfigName = "config.yaml"

// FileConfig 是 config.yaml 的可序列化形态。
//
// 布尔用指针以区分"未设置"与"显式 false"。AdminPassword 仅用于读取（不建议长期保存明文），
// 写入时只落 AdminPasswordHash。
type FileConfig struct {
	HTTPAddr          string
	BaseURL           string
	DBPath            string
	Mode              string
	CookieSecure      *bool
	AppSecret         string
	AdminUsername     string
	AdminPassword     string // 仅读取兼容；不写入
	AdminPasswordHash string

	UpdateCheckEnabled *bool
	UpdateChannel      string
	GitHubRepo         string
}

// ResolveConfigPath 解析配置文件路径：
//
//	MGATE_CONFIG 优先 → ./config.yaml → （非 Windows）/etc/mgate-cloud/config.yaml
//
// 若都不存在，返回默认写入目标 ./config.yaml（供 setup 生成）。
func ResolveConfigPath() string {
	if p := strings.TrimSpace(os.Getenv(envConfigPath)); p != "" {
		return p
	}
	if fileExists(defaultConfigName) {
		return defaultConfigName
	}
	if runtime.GOOS != "windows" {
		etc := "/etc/mgate-cloud/" + defaultConfigName
		if fileExists(etc) {
			return etc
		}
	}
	return defaultConfigName
}

// fileExists 报告路径是否为存在的常规文件。
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// LoadFile 读取并解析配置文件。
func LoadFile(path string) (*FileConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: 打开配置文件失败: %w", err)
	}
	defer f.Close()

	kv := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		val := unquoteYAML(strings.TrimSpace(line[colon+1:]))
		kv[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("config: 读取配置文件失败: %w", err)
	}

	fc := &FileConfig{
		HTTPAddr:           kv["http_addr"],
		BaseURL:            kv["base_url"],
		DBPath:             kv["db_path"],
		Mode:               kv["mode"],
		CookieSecure:       boolPtr(kv, "cookie_secure"),
		AppSecret:          kv["app_secret"],
		AdminUsername:      kv["admin_username"],
		AdminPassword:      kv["admin_password"],
		AdminPasswordHash:  kv["admin_password_hash"],
		UpdateCheckEnabled: boolPtr(kv, "update_check_enabled"),
		UpdateChannel:      kv["update_channel"],
		GitHubRepo:         kv["github_repo"],
	}
	return fc, nil
}

// SaveFile 将配置写入文件，权限 0600（尽量收紧；Windows 上 0600 语义有限）。
//
// 安全：只写 admin_password_hash，绝不写明文密码；app_secret 等敏感值落盘但文件权限收紧。
func SaveFile(path string, fc *FileConfig) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("config: 创建配置目录失败: %w", err)
		}
	}

	var b strings.Builder
	b.WriteString("# mgate-cloud 配置文件（由 Setup 生成）。\n")
	b.WriteString("# 注意：本文件含 app_secret 等敏感信息，请妥善保管、收紧权限（0600）。\n")
	b.WriteString("# 环境变量优先级高于本文件。\n\n")

	writeStr(&b, "http_addr", fc.HTTPAddr)
	writeStr(&b, "base_url", fc.BaseURL)
	writeStr(&b, "db_path", fc.DBPath)
	writeStr(&b, "mode", fc.Mode)
	writeBool(&b, "cookie_secure", fc.CookieSecure)
	b.WriteString("\n# 设备码签名密钥：生产环境必须固定保存；丢失将导致已发设备码校验失败。\n")
	writeStr(&b, "app_secret", fc.AppSecret)
	b.WriteString("\n# 管理员：仅保存口令哈希（bcrypt），绝不保存明文。\n")
	writeStr(&b, "admin_username", fc.AdminUsername)
	writeStr(&b, "admin_password_hash", fc.AdminPasswordHash)
	b.WriteString("\n# 更新检查（GitHub Releases）。\n")
	writeBool(&b, "update_check_enabled", fc.UpdateCheckEnabled)
	writeStr(&b, "update_channel", fc.UpdateChannel)
	writeStr(&b, "github_repo", fc.GitHubRepo)

	// 先写临时文件再原子改名，避免半写损坏。
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("config: 写入配置文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("config: 保存配置文件失败: %w", err)
	}
	// 再次确保权限（某些平台 Rename 后需再 Chmod）。
	_ = os.Chmod(path, 0o600)
	return nil
}

func writeStr(b *strings.Builder, key, val string) {
	if val == "" {
		return
	}
	fmt.Fprintf(b, "%s: %s\n", key, quoteYAML(val))
}

func writeBool(b *strings.Builder, key string, val *bool) {
	if val == nil {
		return
	}
	fmt.Fprintf(b, "%s: %t\n", key, *val)
}

// boolPtr 从扁平 map 解析布尔；缺失返回 nil。
func boolPtr(kv map[string]string, key string) *bool {
	raw, ok := kv[key]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	if b, err := strconv.ParseBool(strings.TrimSpace(raw)); err == nil {
		return &b
	}
	return nil
}

// quoteYAML 用双引号包裹并转义，保证特殊字符安全往返。
func quoteYAML(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// unquoteYAML 去除可选的引号并反转义。
func unquoteYAML(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		inner := s[1 : len(s)-1]
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		return inner
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}
