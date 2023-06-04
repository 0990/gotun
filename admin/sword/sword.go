package sword

import (
	"embed"
	"github.com/0990/gotun/admin/route"
	"github.com/0990/gotun/tun"
	auth "github.com/abbot/go-http-auth"
	_ "github.com/go-sql-driver/mysql"
)

func Run(assets embed.FS, listen string, manager *tun.Manager, digestAuth *auth.DigestAuth) {
	route.Register(assets, listen, manager, digestAuth)
}
