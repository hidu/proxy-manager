package manager

import (
	"encoding/base64"
	"fmt"
	"github.com/hidu/goutils"
	"log"
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

func (user *User) pswEq(psw string) bool {
	return user.PswMd5 == utils.StrMd5(psw)
}

func (user *User) PswEnc() string {
	return utils.StrMd5(fmt.Sprintf("%s:%s", user.Name, user.PswMd5))
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

func loadUsers(confPath string) (users map[string]*User, err error) {
	users = make(map[string]*User)
	if !utils.File_exists(confPath) {
		return
	}
	userInfo_byte, err := utils.File_get_contents(confPath)
	if err != nil {
		log.Println("load user file failed:", confPath, err)
		return
	}
	lines := utils.LoadText2SliceMap(string(userInfo_byte))
	for _, line := range lines {
		name, has := line["name"]
		if !has || name == "" {
			continue
		}
		if _, has := users[name]; has {
			log.Println("dup name in users:", name, line)
			continue
		}

		user := new(User)
		user.Name = name
		if val, has := line["is_admin"]; has && (val == "admin" || val == "true") {
			user.IsAdmin = true
		}
		if val, has := line["psw_md5"]; has {
			user.PswMd5 = val
		}

		if user.PswMd5 == "" {
			if val, has := line["psw"]; has {
				user.Psw = val
				user.PswMd5 = utils.StrMd5(val)
			}
		}
		users[user.Name] = user
	}
	return
}

func (manager *ProxyManager) checkHttpAuth(user *User) bool {
	switch manager.config.authType {
	case AuthType_NO:
		return true
	case AuthType_Basic:
		if u, has := manager.users[user.Name]; has {
			return u.Name == user.Name && u.PswMd5 == user.PswMd5
		}
		return false
	case AuthType_Basic_WithAny:
		return user.Name != ""
		return true
	default:
		return false
	}
	return false
}
