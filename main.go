package main

import (
	"flag"
	"fmt"
	"github.com/hidu/proxy-manager/manager"
	"log"
)

var configPath = flag.String("conf", "./conf/proxy.conf", "proxy's config file")
var initConf = flag.String("init_conf", "", "init conf if not exists")

func main() {
	flag.Usage = func() {
		fmt.Println("proxy manager\n  version:", manager.GetVersion())
		fmt.Println("  https://github.com/hidu/proxy-manager/\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *initConf != "" {
		manager.InitConf(*initConf)
		return
	}
	log.SetFlags(log.Lshortfile | log.LstdFlags | log.Ldate)
	manager := manager.NewProyManager(*configPath)
	manager.Start()
}
