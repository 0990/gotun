package sword

import (
	"github.com/0990/gotun/admin/config"
	"github.com/0990/gotun/admin/route"
	_ "github.com/go-sql-driver/mysql"
)

func Run(conf string) {
	err := config.LoadConfig(conf)
	if err != nil {
		panic(err)
	}
	config.InitDB()
	// Register route
	route.Register()
}
