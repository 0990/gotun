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
	cfg, err := parseAppConfigFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			if err = createAppConfigFile(fileName); err == nil {
				logrus.Infof("cfgfile not exist,create one:%s", fileName)
				return nil
			}
		}
		return fmt.Errorf("parseAppConfigFile fail:%w", err)
	}

	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		return err
	}

	closeLogger, err := InitLog("./logs", "gotun", false, true, 0, level)
	if err != nil {
		return err
	}

	SafeGo(func() {
		if cfg.PProfPort > 0 {
			http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", cfg.PProfPort), nil)
		}
	})

	if cfg.EchoPort > 0 {
		err := echoserver.StartEchoServer(fmt.Sprintf("0.0.0.0:%d", cfg.EchoPort))
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
		if user == cfg.Admin.Username {
			return MD5(cfg.Admin.Username + ":" + realm + ":" + cfg.Admin.Password)
		}
		return ""
	}
	digestAuth := auth.NewDigestAuthenticator(realm, secret)

	// 核心2：启动CRUD服务
	sword.Run(assets, int32(cfg.ListenPort), mgr, digestAuth)

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