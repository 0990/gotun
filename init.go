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

	SafeGo(func() {
		if len(appCfg.PProfListen) > 0 {
			http.ListenAndServe(appCfg.PProfListen, nil)
		}
	})

	SafeGo(func() {
		if len(appCfg.PrometheusListen) > 0 {
			http.Handle("/metrics", promhttp.Handler())
			http.ListenAndServe(appCfg.PrometheusListen, nil)
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

	authMgr := route.NewAuthManager(appCfg.WebUsername, appCfg.WebPassword, appCfg.WebLoginFailLimitInHour)

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
