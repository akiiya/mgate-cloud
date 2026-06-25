// Command mgate-cloud 是设备管理控制面的单体服务入口。
//
// 安全边界（务必牢记并在演进中保持）：
//   - cloud 不 SSH 到设备，不执行 shell，不提供 raw exec / bash -c。
//   - 后续所有设备控制都必须由 mgate-agent 主动连接 cloud，再经白名单 action 执行。
//   - 本进程不引入 os/exec 等任意命令执行能力。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mgate-cloud/internal/app"
	"mgate-cloud/internal/config"
	"mgate-cloud/internal/version"
)

func main() {
	// 统一日志前缀与时间，便于运维排查；日志中不含任何敏感信息。
	log.SetFlags(log.LstdFlags | log.LUTC)
	log.SetPrefix("[mgate-cloud] ")

	// 解析配置：环境变量 > config.yaml > 默认值。无配置文件时进入 setup 模式（由 app 判定）。
	cfg, info, err := config.Resolve()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 启动横幅：版本、模式、监听地址、数据库路径、配置文件——绝不输出 secret。
	log.Printf("mgate-cloud %s 启动: mode=%s addr=%s db=%q config=%q(exists=%t)",
		version.Version, cfg.Mode, cfg.HTTPAddr, cfg.DBPath, info.ConfigPath, info.FileExists)

	// 生产模式硬校验：MGATE_APP_SECRET 必须显式配置，否则拒绝启动。
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置校验失败: %v", err)
	}
	log.Printf("完整配置: %s", cfg) // Config.String 已对所有 secret 脱敏

	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("初始化失败: %v", err)
	}
	defer application.Close()

	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: application.Handler(),
		// 设置基础超时，避免慢连接长期占用资源（公网服务的基本防护）。
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	runServer(server)
}

// runServer 启动 HTTP 服务并处理优雅关闭。
//
// 收到 SIGINT/SIGTERM 后给出有限的关闭窗口，让进行中的请求自然结束，
// 避免硬退出导致连接被粗暴切断。
func runServer(server *http.Server) {
	// 在独立 goroutine 中监听，主 goroutine 负责等待退出信号。
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("HTTP 服务监听于 %s", server.Addr)
		// ListenAndServe 正常关闭时返回 ErrServerClosed，不算异常。
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		log.Fatalf("HTTP 服务异常退出: %v", err)
	case sig := <-stop:
		log.Printf("收到信号 %v，开始优雅关闭…", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("优雅关闭超时/失败: %v", err)
	}
	log.Printf("服务已关闭")
}
