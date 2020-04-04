package main

//go:generate goasset

import (
	"flag"
	"fmt"
	"log"

	"github.com/hidu/proxy-manager/internal"
)

var configPath = flag.String("conf", "./conf/proxy.conf", "proxy's config file")
var initConf = flag.String("init_conf", "", "[conf dir] init conf if not exists")

func main() {
	flag.Usage = func() {
		fmt.Println("proxy manager\n  version:", internal.GetVersion())
		fmt.Printf("  https://github.com/hidu/proxy-manager/\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *initConf != "" {
		internal.InitConf(*initConf)
		return
	}
	log.SetFlags(log.Lshortfile | log.LstdFlags | log.Ldate)
	manager := internal.NewProyManager(*configPath)
	manager.Start()
}
