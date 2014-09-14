package manager

import (
	"encoding/base64"
	"github.com/hidu/goutils"
	"net/http"
	"strings"
)

var proxyAuthorizatonHeader = "Proxy-Authorization"

type User struct {
	Name         string
	Psw          string
	PswMd5       string
	IsAdmin      bool
	SkipCheckPsw bool
}

func getAuthorInfo(req *http.Request) *User {
	defaultInfo := new(User)
	authheader := strings.SplitN(req.Header.Get(proxyAuthorizatonHeader), " ", 2)
	if len(authheader) != 2 || authheader[0] != "Basic" {
		return defaultInfo
	}
	userpassraw, err := base64.StdEncoding.DecodeString(authheader[1])
	if err != nil {
		return defaultInfo
	}
	userpass := strings.SplitN(string(userpassraw), ":", 2)
	if len(userpass) != 2 {
		return defaultInfo
	}
	return &User{Name: userpass[0], PswMd5: utils.StrMd5(userpass[1])}
}
