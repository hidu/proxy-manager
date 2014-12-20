package manager

import (
	"time"
)

var ProxyVersion string = ""
var ServerStartTime time.Time = time.Now()

const TIME_FORMAT_STD string = "2006-01-02 15:04:05"

func init() {
	ProxyVersion = GetVersion()
}

func GetVersion() string {
	return Assest.GetContent("/res/version")
}
