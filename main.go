package main

import (
	"flag"
	"fmt"

	"github.com/xanygo/anygo/xattr"
	"github.com/xanygo/anygo/xcfg"
	"github.com/xanygo/ext"

	"github.com/hidu/proxy-manager/internal"
)

var c = flag.String("conf", "./conf/app.yml", "proxy's config file")

func init() {
	ext.Init()

	flag.Usage = func() {
		fmt.Println("proxy manager\n  version:", internal.GetVersion())
		fmt.Print("  https://github.com/hidu/proxy-manager/\n\n")
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()
	xattr.MustInitAppMain(*c, xcfg.Parse)
	internal.Setup()
	internal.Start()
}
