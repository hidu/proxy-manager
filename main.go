package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/fsgo/fsconf/confext"

	"github.com/hidu/proxy-manager/internal"
)

var configPath = flag.String("conf", "./conf/proxy.toml", "proxy's config file")
var initConf = flag.Bool("init", false, "create config files if not exists")

func init() {
	flag.Usage = func() {
		fmt.Println("proxy manager\n  version:", internal.GetVersion())
		fmt.Print("  https://github.com/hidu/proxy-manager/\n\n")
		flag.PrintDefaults()
	}
	log.SetFlags(log.Lshortfile | log.LstdFlags | log.Ldate)

	confext.Init()
}

func main() {
	flag.Parse()
	if *initConf {
		internal.InitConf(filepath.Dir(*configPath))
		return
	}
	manager := internal.NewProxyManager(*configPath)
	manager.Start()
}
