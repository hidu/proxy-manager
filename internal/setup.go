package internal

import (
	"os"

	"github.com/xanygo/anygo/xattr"
	"github.com/xanygo/anygo/xlog"
)

func Setup() {
	initLogger()
	pool = loadPool()
}

var version = "1.0.20260415"

const timeFormatStd = "2006-01-02 15:04:05"

func GetVersion() string {
	return version
}

func initLogger() {
	logStore := xattr.GetDefault[string]("LogStore", "stderr")
	xlog.DefaultLevel = xlog.LevelDebug

	switch logStore {
	case "file":
		xlog.InitAllDefaultLogger()
	case "no":
		xlog.SetAllDefaultLogger(xlog.NopLogger{})
	default:
		lg := xlog.NewSimpleWithLevel(os.Stderr, xlog.LevelDebug)
		xlog.SetAllDefaultLogger(lg)
	}
}
