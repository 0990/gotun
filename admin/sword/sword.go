package sword

import (
	"embed"
	"github.com/0990/gotun/admin/route"
	"github.com/0990/gotun/server/service"
	"github.com/0990/gotun/tun"
	_ "github.com/go-sql-driver/mysql"
)

func Run(assets embed.FS, listen string, manager *tun.Manager, svcMgr *service.Manager, authMgr *route.AuthManager, version string) {
	route.Register(assets, listen, manager, svcMgr, authMgr, version)
}
