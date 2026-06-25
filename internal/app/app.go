// Package app 负责组装各组件（依赖装配）并暴露可启动的 HTTP 处理器。
//
// 这里是"组合根"（composition root）：在唯一的位置创建并连接 db、store、
// service、handler 与路由，其余包均不持有全局状态，便于测试与演进。
package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"mgate-cloud/internal/admin"
	"mgate-cloud/internal/agent"
	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/auth"
	"mgate-cloud/internal/command"
	"mgate-cloud/internal/config"
	"mgate-cloud/internal/db"
	"mgate-cloud/internal/device"
	"mgate-cloud/internal/hub"
	"mgate-cloud/internal/model"
	"mgate-cloud/internal/util"
	"mgate-cloud/web"

	"net/http"
	"strings"
)

// App 持有应用运行所需的核心资源与已装配的 HTTP 处理器。
type App struct {
	cfg     config.Config
	db      *sql.DB
	hub     *hub.Hub
	cancel  context.CancelFunc // 停止后台 reaper（命令超时清理）
	handler http.Handler
}

// New 按配置装配整个应用：打开数据库、执行迁移、装配服务与路由、bootstrap 管理员。
//
// 任一步骤失败都会返回错误并释放已分配资源，保证不留下半初始化的实例。
func New(cfg config.Config) (*App, error) {
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	if err := db.Migrate(database); err != nil {
		database.Close()
		return nil, fmt.Errorf("app: 执行数据库迁移失败: %w", err)
	}

	// 启动自检：确认 WAL 已生效，并打印（不含任何敏感信息）。
	if mode, err := db.VerifyWAL(database); err == nil {
		log.Printf("app: SQLite journal_mode=%s", mode)
	}

	clock := util.SystemClock{}

	// 装配 store 与 service（依赖自下而上注入）。
	adminStore := auth.NewAdminStore(database, clock)
	sessionStore := auth.NewSessionStore(database, clock)
	authService := auth.NewService(adminStore, sessionStore, cfg.SessionTTL)
	auditService := audit.NewService(database, clock)

	// 在装配完成后处理 bootstrap 管理员（可能写审计日志）。
	if err := bootstrapAdmin(cfg, authService, auditService); err != nil {
		database.Close()
		return nil, err
	}

	// AppSecret 为临时生成时告警：生产环境必须固定且足够随机，
	// 否则重启会使既有设备码全部失效（签名密钥变化）。
	if cfg.AppSecretGenerated {
		log.Printf("app: 警告——未配置 MGATE_APP_SECRET，已生成临时签名密钥；重启将使已发设备码失效，生产环境务必固定配置")
	}

	// 装配设备身份相关 store 与 service（Phase 2/3）。
	deviceStore := device.NewDeviceStore(database)
	credentialStore := device.NewCredentialStore(database)
	pairingStore := device.NewPairingStore(database)
	statusStore := device.NewStatusStore(database)
	codec := device.NewCodec(cfg.AppSecret)

	// Hub 管理 agent WebSocket 连接；同时作为 device.Service 的在线状态来源（Presence）。
	connectionHub := hub.New(cfg.WSOfflineAfter, clock)
	connectionHub.StartReaper(cfg.WSHeartbeatInterval)

	deviceService := device.NewService(database, clock, deviceStore, credentialStore, pairingStore, statusStore, codec, connectionHub, deviceSettings(cfg))

	// 装配命令队列（Phase 4）。Deliverer 由 agent 包基于 Hub 实现，避免 command→agent 导入环。
	commandStore := command.NewStore(database)
	resultStore := command.NewResultStore(database)
	deliverer := agent.NewCommandDeliverer(connectionHub)
	dispatcher := command.NewDispatcher(database, commandStore, deliverer, clock, cfg.CommandLeaseSeconds, cfg.CommandRetryBackoff)
	commandService := command.NewService(database, clock, commandStore, resultStore, dispatcher, deviceService, auditService, command.Settings{
		DefaultTimeout:    cfg.CommandDefaultTimeout,
		MaxTimeout:        cfg.CommandMaxTimeout,
		ResultMaxBytes:    cfg.CommandResultMaxBytes,
		PendingTTL:        cfg.CommandPendingTTL,
		DefaultMaxAttempt: cfg.CommandDefaultMaxAttempts,
		LeaseDuration:     cfg.CommandLeaseSeconds,
		RetryBackoff:      cfg.CommandRetryBackoff,
		PullMaxCommands:   cfg.PullMaxCommands,
	})

	// 启动命令超时/过期清理 goroutine；ctx 在 Close 时取消。
	reaperCtx, cancel := context.WithCancel(context.Background())
	go commandService.RunReaper(reaperCtx, cfg.CommandReaperInterval)

	handlers := admin.NewHandlers(authService, auditService, cfg.CookieSecure, cfg.SessionTTL)
	deviceHandlers := admin.NewDeviceHandlers(deviceService, auditService)
	commandHandlers := admin.NewCommandHandlers(commandService)
	agentHandlers := agent.NewHandlers(deviceService, auditService)
	wsHandlers := agent.NewWSHandlers(deviceService, commandService, auditService, connectionHub, cfg.WSHeartbeatInterval, cfg.WSOfflineAfter, cfg.WSMaxMessageBytes)
	pullHandlers := agent.NewPullHandlers(deviceService, commandService, auditService, cfg.PullMaxBodyBytes, cfg.PullMaxCommands, cfg.PullDefaultInterval)

	distFS, err := web.DistFS()
	if err != nil {
		cancel()
		connectionHub.Stop()
		database.Close()
		return nil, fmt.Errorf("app: 加载内嵌前端资源失败: %w", err)
	}

	handler := buildRoutes(routeDeps{
		auth:       handlers,
		devices:    deviceHandlers,
		commands:   commandHandlers,
		agent:      agentHandlers,
		ws:         wsHandlers,
		pull:       pullHandlers,
		authSvc:    authService,
		distFS:     distFS,
		trustProxy: cfg.TrustProxyHeaders,
		// 就绪探测：数据库可 Ping 即视为就绪。
		readyCheck: func(ctx context.Context) error { return database.PingContext(ctx) },
	})

	return &App{cfg: cfg, db: database, hub: connectionHub, cancel: cancel, handler: handler}, nil
}

// deviceSettings 从全局配置派生设备服务所需的设置快照。
//
// gateway 即对外 BaseURL；ws_url/pull_url 仅作为"预告"返回给 agent，
// Phase 2 并不实现这两个端点（无 WebSocket / Pull）。
func deviceSettings(cfg config.Config) device.Settings {
	return device.Settings{
		PairingTTL:        cfg.PairingTTL,
		DeviceTokenBytes:  cfg.DeviceTokenBytes,
		PairingTokenBytes: cfg.PairingTokenBytes,
		Gateway:           cfg.BaseURL,
		WSURL:             toWebSocketScheme(cfg.BaseURL) + "/api/agent/ws",
		PullURL:           cfg.BaseURL + "/api/agent/pull",
	}
}

// toWebSocketScheme 把 http(s) 基础地址转换为 ws(s) 方案。
func toWebSocketScheme(baseURL string) string {
	switch {
	case strings.HasPrefix(baseURL, "https://"):
		return "wss://" + strings.TrimPrefix(baseURL, "https://")
	case strings.HasPrefix(baseURL, "http://"):
		return "ws://" + strings.TrimPrefix(baseURL, "http://")
	default:
		return baseURL
	}
}

// Handler 返回装配好的 HTTP 处理器，供 http.Server 使用。
func (a *App) Handler() http.Handler { return a.handler }

// Config 返回运行配置（只读用途）。
func (a *App) Config() config.Config { return a.cfg }

// Close 释放资源：停止 reaper 与 Hub（关闭所有 WebSocket 连接），再关闭数据库。
func (a *App) Close() error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.hub != nil {
		a.hub.Stop()
	}
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

// bootstrapAdmin 在满足条件时创建初始管理员，并记录审计。
//
// 规则：
//   - 未提供 MGATE_ADMIN_USERNAME/PASSWORD：跳过；若库中也无管理员则告警（将无法登录）。
//   - 已存在管理员：不重复创建（幂等）。
//   - 真正创建成功：记录 system.bootstrap_admin.created 审计，并打印不含口令的日志。
func bootstrapAdmin(cfg config.Config, authService *auth.Service, auditService *audit.Service) error {
	ctx := context.Background()

	if !cfg.HasBootstrapAdmin() {
		count, err := authService.AdminCount(ctx)
		if err != nil {
			return err
		}
		if count == 0 {
			log.Printf("app: 警告——系统中尚无管理员，且未提供 MGATE_ADMIN_USERNAME/PASSWORD，当前无法登录")
		}
		return nil
	}

	created, admin, err := authService.BootstrapAdmin(ctx, cfg.AdminUsername, cfg.AdminPassword)
	if err != nil {
		return fmt.Errorf("app: 创建 bootstrap 管理员失败: %w", err)
	}
	if !created {
		log.Printf("app: 已存在管理员，跳过 bootstrap")
		return nil
	}

	log.Printf("app: 已创建 bootstrap 管理员 username=%q", admin.Username)
	auditService.Record(ctx, audit.Entry{
		ActorType: model.ActorTypeSystem,
		Action:    model.ActionBootstrapAdminCreated,
		ActorID:   admin.ID,
		Summary:   "创建初始管理员账户",
		Metadata:  map[string]any{"username": admin.Username},
	})
	return nil
}
