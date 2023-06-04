package gotun

import (
	"crypto/md5"
	"embed"
	"encoding/hex"
	"fmt"
	"github.com/0990/gotun/admin/sword"
	"github.com/0990/gotun/echoserver"
	"github.com/0990/gotun/tun"
	auth "github.com/abbot/go-http-auth"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

//go:embed admin/resource
//go:embed admin/view
var assets embed.FS

func Run(fileName string) error {
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

	if len(appCfg.EchoListen) > 0 {
		err := echoserver.StartEchoServer(appCfg.EchoListen)
		if err != nil {
			return err
		}
	}

	mgr := tun.NewManager()
	err = mgr.Run()
	if err != nil {
		return err
	}

	realm := "example.com"
	secret := func(user, realm string) string {
		if user == appCfg.WebUsername {
			return MD5(appCfg.WebUsername + ":" + realm + ":" + appCfg.WebPassword)
		}
		return ""
	}
	digestAuth := auth.NewDigestAuthenticator(realm, secret)

	// 核心2：启动CRUD服务
	sword.Run(assets, appCfg.WebListen, mgr, digestAuth)

	Welcome(appCfg)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, os.Kill, syscall.SIGTERM)
	signal := <-quit
	fmt.Printf("receive signal %v,quit... \n", signal)

	closeLogger()
	return nil
}

func MD5Bytes(s []byte) string {
	ret := md5.Sum(s)
	return hex.EncodeToString(ret[:])
}

// 计算字符串MD5值
func MD5(s string) string {
	return MD5Bytes([]byte(s))
}
