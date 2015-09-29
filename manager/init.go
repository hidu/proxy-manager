package manager

import (
	"time"
)

var proxyVersion = ""

var serverStartTime = time.Now()

const timeFormatStd = "2006-01-02 15:04:05"

func init() {
	proxyVersion = GetVersion()
}

// GetVersion version
func GetVersion() string {
	return Assest.GetContent("/res/version")
}
