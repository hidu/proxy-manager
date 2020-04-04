package internal

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/hidu/goutils/fs"
	"github.com/hidu/goutils/str_util"
)

var proxyAuthorizatonHeader = "Proxy-Authorization"

type user struct {
	Name         string
	Psw          string
	PswMd5       string
	IsAdmin      bool
	SkipCheckPsw bool
}

func (user *user) pswEq(psw string) bool {
	return user.PswMd5 == str_util.StrMd5(psw)
}

func (user *user) PswEnc() string {
	return str_util.StrMd5(fmt.Sprintf("%s:%s", user.Name, user.PswMd5))
}
func (user *user) Eq(u *user) bool {
	return u != nil && user.Name == u.Name && u.PswMd5 == user.PswMd5
}

func (user *user) String() string {
	return fmt.Sprint("name:", user.Name, ",psw:", user.Psw, ",pswMd5:", user.PswMd5, ",isAdmin:", user.IsAdmin)
}

func getAuthorInfo(req *http.Request) *user {
	defaultInfo := new(user)
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
	return &user{Name: userpass[0], PswMd5: str_util.StrMd5(userpass[1])}
}

var defaultTestUserName = "_test_"

var defaultTestUser = &user{
	Name:   defaultTestUserName,
	Psw:    fmt.Sprintf("%d", serverStartTime.UnixNano()),
	PswMd5: str_util.StrMd5(fmt.Sprintf("%d", serverStartTime.UnixNano())),
}

func loadUsers(confPath string) (users map[string]*user, err error) {
	users = make(map[string]*user)
	if !fs.FileExists(confPath) {
		return
	}
	userInfoByte, err := fs.FileGetContents(confPath)
	if err != nil {
		log.Println("load user file failed:", confPath, err)
		return
	}
	lines := str_util.LoadText2SliceMap(string(userInfoByte))
	for _, line := range lines {
		name, has := line["name"]
		if !has || name == "" {
			continue
		}
		if _, has := users[name]; has {
			log.Println("dup name in users:", name, line)
			continue
		}

		user := new(user)
		user.Name = name
		if val, has := line["is_admin"]; has && (val == "admin" || val == "true") {
			user.IsAdmin = true
		}
		if val, has := line["psw_md5"]; has {
			user.PswMd5 = strings.TrimSpace(val)
		}

		if user.PswMd5 == "" {
			if val, has := line["psw"]; has {
				user.Psw = strings.TrimSpace(val)
				user.PswMd5 = str_util.StrMd5(user.Psw)
			}
		}
		if user.PswMd5 == "" {
			log.Println("ignore user:", name, "empty passwd")
			continue
		}
		users[user.Name] = user
	}
	return
}

func (manager *ProxyManager) checkHTTPAuth(user *user) bool {
	switch manager.config.authType {
	case AuthTypeNO:
		return true
	case AuthTypeBasic:
		if u, has := manager.users[user.Name]; has {
			return u.Eq(user)
		}
		if defaultTestUser.Eq(user) {
			return true
		}
		return false
	case AuthTypeBasicWithAny:
		return user.Name != ""
	default:
		return false
	}
	return false
}
