package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/hidu/proxy-manager/internal"
)

var configPath = flag.String("conf", "./conf/proxy.toml", "proxy's config file")
var initConf = flag.Bool("init", false, "create config files if not exists")

func main() {
	flag.Usage = func() {
		fmt.Println("proxy manager\n  version:", internal.GetVersion())
		fmt.Printf("  https://github.com/hidu/proxy-manager/\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *initConf {
		internal.InitConf(filepath.Dir(*configPath))
		return
	}
	log.SetFlags(log.Lshortfile | log.LstdFlags | log.Ldate)
	manager := internal.NewProxyManager(*configPath)
	manager.Start()
}
