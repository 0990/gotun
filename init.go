package gotun

import (
	"embed"
	"fmt"
	"github.com/0990/gotun/admin/route"
	"github.com/0990/gotun/admin/sword"
	"github.com/0990/gotun/server/service"
	"github.com/0990/gotun/tun"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
)

//go:embed admin/resource
//go:embed admin/view
var assets embed.FS

func Run(fileName string, tunDir string) error {
	appCfg, err := parseAppConfigFile(fileName)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("parseAppConfigFile fail:%w", err)
		}

		v, err := createAppConfigFile(fileName)
		if err != nil {
			return fmt.Errorf("createAppConfigFile fail:%w", err)
		}

		logrus.Infof("cfgfile not exist,create one:%s", fileName)
		appCfg = v
	}

	level, err := logrus.ParseLevel(appCfg.LogLevel)
	if err != nil {
		return err
	}

	closeLogger, err := InitLog("./logs", "gotun", false, true, 0, level)
	if err != nil {
		return err
	}

	// pprof与prometheus共用同一个监听端口,通过路径区分:
	// /metrics 暴露prometheus指标,/debug/pprof/* 暴露pprof。metrics_listen为空则不开启。
	SafeGo(func() {
		if addr := appCfg.MetricsListen; len(addr) > 0 {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			mux.HandleFunc("GET /debug/pprof/", pprof.Index)
			mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
			mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
			mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
			mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
			mux.Handle("GET /debug/pprof/allocs", pprof.Handler("allocs"))
			mux.Handle("GET /debug/pprof/block", pprof.Handler("block"))
			mux.Handle("GET /debug/pprof/goroutine", pprof.Handler("goroutine"))
			mux.Handle("GET /debug/pprof/heap", pprof.Handler("heap"))
			mux.Handle("GET /debug/pprof/mutex", pprof.Handler("mutex"))
			mux.Handle("GET /debug/pprof/threadcreate", pprof.Handler("threadcreate"))
			http.ListenAndServe(addr, mux)
		}
	})

	mgr := tun.NewManager(tunDir)
	err = mgr.Run()
	if err != nil {
		return err
	}

	// 内置服务实例管理（echo/http_proxy/socks5/socks5x，配置存于 tunnel 目录 .service 文件）
	svcMgr := service.NewManager(tunDir)

	// 旧版 app.yaml 的 build-in 配置一次性迁移为 .service 实例，并从 app.yaml 移除该块
	if err := migrateBuildInConfig(fileName, svcMgr); err != nil {
		logrus.WithError(err).Warn("migrate build-in config failed")
	}

	if err := svcMgr.Run(); err != nil {
		return err
	}

	authMgr := route.NewAuthManager(fileName, appCfg.WebUsername, appCfg.WebPassword, appCfg.WebLoginFailLimitInHour)

	// 核心2：启动CRUD服务
	sword.Run(assets, appCfg.WebListen, mgr, svcMgr, authMgr, Version)

	Welcome(appCfg)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, os.Kill, syscall.SIGTERM)
	signal := <-quit
	fmt.Printf("receive signal %v,quit... \n", signal)

	closeLogger()
	return nil
}
