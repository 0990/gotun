package gotun

import (
	"embed"
	"fmt"
	"github.com/0990/gotun/admin/route"
	"github.com/0990/gotun/admin/sword"
	"github.com/0990/gotun/server/echo"
	"github.com/0990/gotun/server/socks5x"
	"github.com/0990/gotun/tun"
	"github.com/0990/httpproxy"
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

	err = startBuildInServer(appCfg.BuildIn)
	if err != nil {
		return fmt.Errorf("startBuildInServer fail:%w", err)
	}

	mgr := tun.NewManager(tunDir)
	err = mgr.Run()
	if err != nil {
		return err
	}

	authMgr := route.NewAuthManager(fileName, appCfg.WebUsername, appCfg.WebPassword, appCfg.WebLoginFailLimitInHour)

	// 核心2：启动CRUD服务
	sword.Run(assets, appCfg.WebListen, mgr, authMgr, Version)

	Welcome(appCfg)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, os.Kill, syscall.SIGTERM)
	signal := <-quit
	fmt.Printf("receive signal %v,quit... \n", signal)

	closeLogger()
	return nil
}

func startBuildInServer(in BuiltIn) error {
	if !in.Enable {
		return nil
	}

	if len(in.EchoListen) > 0 {
		err := echo.StartEchoServer(in.EchoListen)
		if err != nil {
			return err
		}
	}

	if len(in.HttpProxyListen) > 0 {
		s := httpproxy.NewServer(httpproxy.Config{
			BindAddr: in.HttpProxyListen,
			Hosts:    []string{"*"},
			Verbose:  false,
		})

		go s.ListenAndServe()
	}

	if in.Socks5XServer.ListenPort > 0 {
		s, err := socks5x.NewServer(in.Socks5XServer.ListenPort, in.Socks5XServer.TCPTimeout, in.Socks5XServer.UDPTimout)
		if err != nil {
			return err
		}
		err = s.Run()
		if err != nil {
			return err
		}
	}

	return nil
}
