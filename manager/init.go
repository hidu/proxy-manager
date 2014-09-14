package manager

import (
	"github.com/hidu/goutils"
)

var ProxyVersion string = ""

const TIME_FORMAT_STD string = "2006-01-02 15:04:05"

func init() {

	utils.ResetDefaultBundle()
	ProxyVersion = GetVersion()
}

func GetVersion() string {
	return string(utils.DefaultResource.Load("/res/version"))
}
