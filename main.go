package main

import (
	"fmt"
	"github.com/0990/gotun/admin/sword"
	gosword "github.com/sunshinev/go-sword/v2"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// 这里我们可以调整日志的详细程度
	log.SetFlags(log.Llongfile | log.Ldate)
	gosword.Init("config/go-sword.yaml").Run()

	// 核心2：启动CRUD服务
	sword.Run("config/go-sword.yaml")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, os.Kill, syscall.SIGTERM)
	signal := <-quit
	fmt.Printf("receive signal %v,quit... \n", signal)
}
