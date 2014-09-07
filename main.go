package main

import (
	"./manager"
	"flag"
	"log"
)

var configPath = flag.String("conf", "./conf/proxy.conf", "proxy's config file")

func main() {
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.LstdFlags | log.Ldate)
	manager := manager.NewProyManager(*configPath)
	manager.Start()
}
