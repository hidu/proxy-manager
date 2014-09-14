package manager

import (
	"github.com/hidu/goutils"
)

var ProxyVersion string = ""

func init() {

	utils.ResetDefaultBundle()
	ProxyVersion = GetVersion()
}

func GetVersion() string {
	return string(utils.DefaultResource.Load("/res/version"))
}
