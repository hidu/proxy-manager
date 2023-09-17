package internal

import (
	"time"
)

var version = "0.3.20230917"

var serverStartTime = time.Now()

const timeFormatStd = "2006-01-02 15:04:05"

func GetVersion() string {
	return version
}
