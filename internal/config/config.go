// Package config 负责从环境变量加载运行配置，并提供安全的默认值。
package config

import (
	"fmt"
	"log"
	"strings"
	"time"

	"mgate-cloud/internal/util"
)

// Config 是 mgate-cloud 的全部运行期配置。
//
// 所有字段均可由环境变量覆盖，且都带有合理默认值，保证"零配置"也能本地启动。
type Config struct {
	// Mode 是运行模式：dev / test / prod（production）。
	// 影响安全默认值：prod 下 MGATE_APP_SECRET 为空将拒绝启动；dev/test 可临时生成。
	Mode string
	// HTTPAddr 是 HTTP 服务监听地址，例如 ":8080"。
	HTTPAddr string
	// DBPath 是 SQLite 数据库文件路径。
	DBPath string
	// BaseURL 是对外访问的基础地址，用于后续生成绝对链接（Phase 1 暂仅记录）。
	BaseURL string
	// CookieSecure 控制下发的 cookie 是否带 Secure 属性。
	// 本地 http 开发置 false；公网 https（经 Cloudflare）部署务必置 true。
	CookieSecure bool
	// TrustProxyHeaders 为兼容旧行为的“无条件信任任意对端转发头”开关（blanket）。
	// 一般无需开启：默认即会信任来自本地回环/私有网段（典型反代位置）的转发头，
	// 见 TrustedProxies。仅在反代位于非私有地址且无法用 TrustedProxies 表达时才置 true。
	TrustProxyHeaders bool
	// TrustedProxies 是额外可信代理网段（逗号分隔的 CIDR / IP），追加到默认的
	// 本地回环 + 私有网段之上；用于反代处于公网地址（如自建公网反代、特定 CDN 段）的场景。
	// 特殊值 "none"/"off" 表示谁都不信任（仅用 RemoteAddr，适用于程序直接暴露公网且无反代）。
	TrustedProxies string

	// AdminUsername / AdminPassword 用于首次启动时 bootstrap 管理员。
	// 仅在系统中尚无任何管理员时生效，且明文口令绝不写入日志。
	AdminUsername string
	AdminPassword string
	// AdminPasswordHash 为预先计算好的 bcrypt 口令哈希（来自 config.yaml）。
	// 与 AdminPassword 二选一；优先使用 hash，避免明文落盘。
	AdminPasswordHash string

	// ConfigPath 是解析到的配置文件路径（用于 setup 生成/写回）。
	ConfigPath string
	// ConfigFileExists 标识启动时配置文件是否存在（用于判定是否进入 setup 模式）。
	ConfigFileExists bool

	// SessionTTL 是会话有效期。
	SessionTTL time.Duration

	// --- 登录失败限流（在线暴力破解防护）---

	// LoginThrottleEnabled 是否启用按 IP 的登录失败限流。
	LoginThrottleEnabled bool
	// LoginMaxFailures 触发封禁的窗口内连续失败次数阈值。
	LoginMaxFailures int
	// LoginFailureWindow 连续失败的计数窗口，超过则重置连续计数。
	LoginFailureWindow time.Duration
	// LoginBanBase 首次封禁时长；后续每次封禁按等级翻倍升级。
	LoginBanBase time.Duration
	// LoginBanMax 封禁时长上限。
	LoginBanMax time.Duration
	// LoginBanOffenseReset 距上次失败超过该时长后，封禁等级衰减归零。
	LoginBanOffenseReset time.Duration

	// --- Phase 7：更新检查 / 自更新 ---

	// UpdateCheckEnabled 控制是否允许检查更新。
	UpdateCheckEnabled bool
	// UpdateChannel 更新通道：stable / rc。
	UpdateChannel string
	// GitHubRepo 更新来源仓库（owner/repo）。
	GitHubRepo string

	// --- Phase 2：设备身份相关 ---

	// PairingTTL 是一次性设备码的有效期。
	PairingTTL time.Duration
	// DeviceTokenBytes 是设备长期令牌的随机字节数（熵）。
	DeviceTokenBytes int
	// PairingTokenBytes 是 pairing 令牌的随机字节数（熵）。
	PairingTokenBytes int
	// AppSecret 用于对设备码做 HMAC-SHA256 签名。
	// 生产环境必须固定且足够随机；为空时启动期会生成临时 secret 并告警。
	AppSecret string
	// AppSecretGenerated 标记 AppSecret 是否为本次启动临时生成（仅用于日志告警）。
	AppSecretGenerated bool

	// --- Phase 3：Agent WebSocket 相关 ---

	// WSHeartbeatInterval 是建议 agent 发送心跳的间隔（下发给 agent 参考）。
	WSHeartbeatInterval time.Duration
	// WSOfflineAfter 是判定离线的阈值：超过该时长无心跳即视为离线，并清理连接。
	WSOfflineAfter time.Duration
	// WSMaxMessageBytes 是单条 WebSocket 消息的大小上限。
	WSMaxMessageBytes int64

	// --- Phase 4：命令队列相关 ---

	// CommandDefaultTimeout 是命令默认执行超时。
	CommandDefaultTimeout time.Duration
	// CommandMaxTimeout 是命令允许的最大超时（创建时上限）。
	CommandMaxTimeout time.Duration
	// CommandResultMaxBytes 是 stdout/stderr/result_json 各自的字节上限，超出截断。
	CommandResultMaxBytes int
	// CommandPendingTTL 是命令在 pending 状态的最长存活时间，超时标记 expired。
	CommandPendingTTL time.Duration
	// CommandReaperInterval 是命令超时/过期扫描间隔。
	CommandReaperInterval time.Duration

	// --- Phase 5：HTTPS Pull 兜底与命令重试相关 ---

	// PullDefaultInterval 是建议 agent 下次 Pull 的间隔（下发给 agent 参考）。
	PullDefaultInterval time.Duration
	// PullMaxCommands 是单次 Pull 最多返回的命令数上限。
	PullMaxCommands int
	// PullMaxBodyBytes 是 Pull 请求体大小上限。
	PullMaxBodyBytes int64
	// CommandDefaultMaxAttempts 是命令默认最大投递尝试次数。
	CommandDefaultMaxAttempts int
	// CommandLeaseSeconds 是命令投递租约时长：领取后多久未完成即可被重新投递。
	CommandLeaseSeconds time.Duration
	// CommandRetryBackoff 是重试前的最小等待，避免立即重复投递。
	CommandRetryBackoff time.Duration
}

// 各配置项对应的环境变量名，集中声明便于文档与代码一致。
const (
	envConfigPath     = "MGATE_CONFIG"
	envMode           = "MGATE_MODE"
	envTrustProxy     = "MGATE_TRUST_PROXY_HEADERS"
	envTrustedProxies = "MGATE_TRUSTED_PROXIES"
	envHTTPAddr       = "MGATE_HTTP_ADDR"
	envDBPath         = "MGATE_DB_PATH"
	envBaseURL        = "MGATE_BASE_URL"
	envCookieSecure   = "MGATE_COOKIE_SECURE"

	envUpdateEnabled     = "MGATE_UPDATE_CHECK_ENABLED"
	envUpdateChannel     = "MGATE_UPDATE_CHANNEL"
	envGitHubRepo        = "MGATE_GITHUB_REPO"
	defaultUpdateChannel = "stable"
	defaultGitHubRepo    = "akiiya/mgate-cloud"

	// 运行模式取值。
	ModeDev             = "dev"
	ModeTest            = "test"
	ModeProd            = "prod"
	envAdminUsername    = "MGATE_ADMIN_USERNAME"
	envAdminPassword    = "MGATE_ADMIN_PASSWORD"
	envSessionTTLHours  = "MGATE_SESSION_TTL_HOURS"
	defaultSessionHours = 168 // 7 天

	envLoginThrottleEnabled = "MGATE_LOGIN_THROTTLE_ENABLED"
	envLoginMaxFailures     = "MGATE_LOGIN_MAX_FAILURES"
	envLoginWindowMinutes   = "MGATE_LOGIN_FAILURE_WINDOW_MINUTES"
	envLoginBanBaseMinutes  = "MGATE_LOGIN_BAN_BASE_MINUTES"
	envLoginBanMaxHours     = "MGATE_LOGIN_BAN_MAX_HOURS"
	envLoginBanResetHours   = "MGATE_LOGIN_BAN_RESET_HOURS"
	defaultLoginMaxFailures = 5
	defaultLoginWindowMin   = 15
	defaultLoginBanBaseMin  = 60 // 首次封禁 1 小时
	defaultLoginBanMaxHours = 24 // 封禁上限 24 小时
	defaultLoginBanResetH   = 24 // 24 小时无失败后封禁等级归零

	envPairingTTLMinutes  = "MGATE_PAIRING_TTL_MINUTES"
	envDeviceTokenBytes   = "MGATE_DEVICE_TOKEN_BYTES"
	envPairingTokenBytes  = "MGATE_PAIRING_TOKEN_BYTES"
	envAppSecret          = "MGATE_APP_SECRET"
	defaultPairingMinutes = 30
	// 32 字节（256-bit）是令牌熵的下限：足够抵御暴力枚举，且不允许被弱化。
	defaultTokenBytes = 32
	// minTokenBytes 防止把令牌配置成短码而削弱安全性。
	minTokenBytes = 16
	// appSecretBytes 是临时生成 AppSecret 时的字节数。
	appSecretBytes = 32

	envWSHeartbeatSec     = "MGATE_WS_HEARTBEAT_INTERVAL_SEC"
	envWSOfflineAfterSec  = "MGATE_WS_OFFLINE_AFTER_SEC"
	envWSMaxMessageBytes  = "MGATE_WS_MAX_MESSAGE_BYTES"
	defaultWSHeartbeatSec = 25
	defaultWSOfflineSec   = 90
	defaultWSMaxMsgBytes  = 65536

	envCmdDefaultTimeoutSec = "MGATE_COMMAND_DEFAULT_TIMEOUT_SEC"
	envCmdMaxTimeoutSec     = "MGATE_COMMAND_MAX_TIMEOUT_SEC"
	envCmdResultMaxBytes    = "MGATE_COMMAND_RESULT_MAX_BYTES"
	envCmdPendingTTLMin     = "MGATE_COMMAND_PENDING_TTL_MINUTES"
	envCmdReaperIntervalSec = "MGATE_COMMAND_REAPER_INTERVAL_SEC"
	defaultCmdTimeoutSec    = 60
	defaultCmdMaxTimeoutSec = 300
	defaultCmdResultBytes   = 262144 // 256 KiB
	defaultCmdPendingTTLMin = 10
	defaultCmdReaperSec     = 10

	envPullIntervalSec   = "MGATE_PULL_DEFAULT_INTERVAL_SEC"
	envPullMaxCommands   = "MGATE_PULL_MAX_COMMANDS"
	envPullMaxBodyBytes  = "MGATE_PULL_MAX_BODY_BYTES"
	envCmdMaxAttempts    = "MGATE_COMMAND_DEFAULT_MAX_ATTEMPTS"
	envCmdLeaseSec       = "MGATE_COMMAND_LEASE_SECONDS"
	envCmdRetryBackoff   = "MGATE_COMMAND_RETRY_BACKOFF_SEC"
	defaultPullIntervalS = 15
	defaultPullMaxCmds   = 10
	defaultPullBodyBytes = 131072 // 128 KiB
	defaultCmdMaxAttempt = 3
	defaultCmdLeaseSec   = 60
	defaultCmdBackoffSec = 10
)

// Load 从环境变量构建 Config（不读取配置文件）。保留供测试与纯 env 场景使用。
func Load() Config {
	return loadInternal(&FileConfig{})
}

// Resolve 解析配置：定位配置文件 → 以"环境变量 > 文件 > 默认值"的优先级构建 Config。
//
// 返回的 ResolveInfo 标识配置文件是否存在（供 setup 判定）。
func Resolve() (Config, ResolveInfo, error) {
	path := ResolveConfigPath()
	info := ResolveInfo{ConfigPath: path, FileExists: fileExists(path)}

	fc := &FileConfig{}
	if info.FileExists {
		loaded, err := LoadFile(path)
		if err != nil {
			return Config{}, info, err
		}
		fc = loaded
	}

	cfg := loadInternal(fc)
	cfg.ConfigPath = path
	cfg.ConfigFileExists = info.FileExists
	return cfg, info, nil
}

// ResolveInfo 描述配置解析结果。
type ResolveInfo struct {
	ConfigPath string
	FileExists bool
}

// loadInternal 以"环境变量 > 文件 > 默认值"的优先级构建 Config。
//
// 该函数不做"致命校验"——所有项都有默认值，总能返回一份可用配置。
func loadInternal(fc *FileConfig) Config {
	cfg := Config{
		Mode:              normalizeMode(pickStr(envMode, fc.Mode, ModeDev)),
		HTTPAddr:          pickStr(envHTTPAddr, fc.HTTPAddr, ":8080"),
		DBPath:            pickStr(envDBPath, fc.DBPath, "./data/mgate-cloud.db"),
		BaseURL:           pickStr(envBaseURL, fc.BaseURL, "http://127.0.0.1:8080"),
		CookieSecure:      pickBool(envCookieSecure, fc.CookieSecure, false),
		TrustProxyHeaders: pickBool(envTrustProxy, fc.TrustProxyHeaders, false),
		TrustedProxies:    envString(envTrustedProxies, ""),
		AdminUsername:     pickStr(envAdminUsername, fc.AdminUsername, ""),
		AdminPassword:     pickStr(envAdminPassword, fc.AdminPassword, ""),
		AdminPasswordHash: fc.AdminPasswordHash,
		SessionTTL:        time.Duration(envInt(envSessionTTLHours, defaultSessionHours)) * time.Hour,

		LoginThrottleEnabled: pickBool(envLoginThrottleEnabled, nil, true),
		LoginMaxFailures:     max(envInt(envLoginMaxFailures, defaultLoginMaxFailures), 1),
		LoginFailureWindow:   time.Duration(envInt(envLoginWindowMinutes, defaultLoginWindowMin)) * time.Minute,
		LoginBanBase:         time.Duration(envInt(envLoginBanBaseMinutes, defaultLoginBanBaseMin)) * time.Minute,
		LoginBanMax:          time.Duration(envInt(envLoginBanMaxHours, defaultLoginBanMaxHours)) * time.Hour,
		LoginBanOffenseReset: time.Duration(envInt(envLoginBanResetHours, defaultLoginBanResetH)) * time.Hour,

		UpdateCheckEnabled: pickBool(envUpdateEnabled, fc.UpdateCheckEnabled, true),
		UpdateChannel:      pickStr(envUpdateChannel, fc.UpdateChannel, defaultUpdateChannel),
		GitHubRepo:         pickStr(envGitHubRepo, fc.GitHubRepo, defaultGitHubRepo),

		PairingTTL:        time.Duration(envInt(envPairingTTLMinutes, defaultPairingMinutes)) * time.Minute,
		DeviceTokenBytes:  tokenBytes(envDeviceTokenBytes),
		PairingTokenBytes: tokenBytes(envPairingTokenBytes),

		WSHeartbeatInterval: time.Duration(envInt(envWSHeartbeatSec, defaultWSHeartbeatSec)) * time.Second,
		WSOfflineAfter:      time.Duration(envInt(envWSOfflineAfterSec, defaultWSOfflineSec)) * time.Second,
		WSMaxMessageBytes:   int64(envInt(envWSMaxMessageBytes, defaultWSMaxMsgBytes)),

		CommandDefaultTimeout: time.Duration(envInt(envCmdDefaultTimeoutSec, defaultCmdTimeoutSec)) * time.Second,
		CommandMaxTimeout:     time.Duration(envInt(envCmdMaxTimeoutSec, defaultCmdMaxTimeoutSec)) * time.Second,
		CommandResultMaxBytes: envInt(envCmdResultMaxBytes, defaultCmdResultBytes),
		CommandPendingTTL:     time.Duration(envInt(envCmdPendingTTLMin, defaultCmdPendingTTLMin)) * time.Minute,
		CommandReaperInterval: time.Duration(envInt(envCmdReaperIntervalSec, defaultCmdReaperSec)) * time.Second,

		PullDefaultInterval:       time.Duration(envInt(envPullIntervalSec, defaultPullIntervalS)) * time.Second,
		PullMaxCommands:           envInt(envPullMaxCommands, defaultPullMaxCmds),
		PullMaxBodyBytes:          int64(envInt(envPullMaxBodyBytes, defaultPullBodyBytes)),
		CommandDefaultMaxAttempts: envInt(envCmdMaxAttempts, defaultCmdMaxAttempt),
		CommandLeaseSeconds:       time.Duration(envInt(envCmdLeaseSec, defaultCmdLeaseSec)) * time.Second,
		CommandRetryBackoff:       time.Duration(envInt(envCmdRetryBackoff, defaultCmdBackoffSec)) * time.Second,
	}

	// AppSecret 处理：
	//   - prod：为空时【不】生成，保留空值，交由 Validate 拒绝启动（安全硬要求）。
	//   - dev/test：为空时临时生成，保证开箱即用；并标记 generated 以便告警。
	cfg.AppSecret = pickStr(envAppSecret, fc.AppSecret, "")
	if cfg.AppSecret == "" && !cfg.IsProduction() {
		if generated, err := util.RandomToken(appSecretBytes); err == nil {
			cfg.AppSecret = generated
			cfg.AppSecretGenerated = true
		} else {
			// 极端情况下系统熵不可用：这是不可恢复的安全前提缺失。
			log.Fatalf("config: 无法生成临时 AppSecret: %v", err)
		}
	}

	return cfg
}

// normalizeMode 规整运行模式，未知值回退为 prod（最安全的默认）。
//
// 说明：环境变量未显式提供时，Load 已用 ModeDev 作为缺省传入；这里只处理"提供了但拼写异常"
// 的情况——此时按最安全策略当作 prod 处理，避免误把生产当 dev 而放过空 AppSecret。
func normalizeMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case ModeDev, "development":
		return ModeDev
	case ModeTest:
		return ModeTest
	case ModeProd, "production":
		return ModeProd
	default:
		return ModeProd
	}
}

// IsProduction 报告是否运行于生产模式。
func (c Config) IsProduction() bool { return c.Mode == ModeProd }

// Validate 做启动期致命校验：返回非 nil 时上层应拒绝启动。
//
// 当前唯一硬约束：生产模式必须显式配置 MGATE_APP_SECRET（不可临时生成），
// 否则重启会使已发设备码失效、且密钥可被推断风险更高。
func (c Config) Validate() error {
	if c.IsProduction() && c.AppSecret == "" {
		return fmt.Errorf("config: 生产模式（MGATE_MODE=prod）必须设置 MGATE_APP_SECRET")
	}
	return nil
}

// tokenBytes 读取令牌字节数配置，并强制不低于安全下限，避免被无意或有意弱化。
func tokenBytes(envKey string) int {
	n := envInt(envKey, defaultTokenBytes)
	if n < minTokenBytes {
		log.Printf("config: %s=%d 低于安全下限 %d，已回退为 %d", envKey, n, minTokenBytes, defaultTokenBytes)
		return defaultTokenBytes
	}
	return n
}

// HasBootstrapAdmin 表示是否提供了用于初始化管理员的用户名与口令。
func (c Config) HasBootstrapAdmin() bool {
	return c.AdminUsername != "" && c.AdminPassword != ""
}

// String 实现 fmt.Stringer，用于安全地打印配置。
//
// 关键点：绝不输出 AdminPassword 明文，避免敏感信息进入启动日志。
func (c Config) String() string {
	return fmt.Sprintf(
		"Config{Mode:%q HTTPAddr:%q DBPath:%q BaseURL:%q CookieSecure:%t TrustProxyHeaders:%t AdminUsername:%q AdminPassword:%s SessionTTL:%s "+
			"PairingTTL:%s DeviceTokenBytes:%d PairingTokenBytes:%d AppSecret:%s "+
			"WSHeartbeatInterval:%s WSOfflineAfter:%s WSMaxMessageBytes:%d}",
		c.Mode, c.HTTPAddr, c.DBPath, c.BaseURL, c.CookieSecure, c.TrustProxyHeaders, c.AdminUsername, redactedSecret(c.AdminPassword), c.SessionTTL,
		c.PairingTTL, c.DeviceTokenBytes, c.PairingTokenBytes, redactedSecret(c.AppSecret),
		c.WSHeartbeatInterval, c.WSOfflineAfter, c.WSMaxMessageBytes,
	)
}

// redactedSecret 将口令渲染为是否存在的占位符，绝不回显明文。
func redactedSecret(s string) string {
	if s == "" {
		return "(unset)"
	}
	return "(set)"
}
