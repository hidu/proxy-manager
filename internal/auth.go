package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/fsgo/fsconf"
)

const proxyAuthorizationHeader = "Proxy-Authorization"

type User struct {
	Name        string
	Password    string
	PasswordMd5 string
	Admin       bool
}

func (u *User) pswEq(psw string) bool {
	return u.PasswordMd5 == StrMd5(psw)
}

func (u *User) PswEnc() string {
	return StrMd5(fmt.Sprintf("%s:%s", u.Name, u.PasswordMd5))
}

func (u *User) Eq(u1 *User) bool {
	return u != nil && u.Name == u1.Name && u.PasswordMd5 == u1.PasswordMd5
}

func (u *User) String() string {
	bf, _ := json.Marshal(u)
	return string(bf)
}

func getProxyAuthorInfo(req *http.Request) *User {
	defaultInfo := new(User)
	authHeader := strings.SplitN(req.Header.Get(proxyAuthorizationHeader), " ", 2)
	if len(authHeader) != 2 || authHeader[0] != "Basic" {
		return defaultInfo
	}
	userPassRaw, err := base64.StdEncoding.DecodeString(authHeader[1])
	if err != nil {
		return defaultInfo
	}
	userPass := strings.SplitN(string(userPassRaw), ":", 2)
	if len(userPass) != 2 {
		return defaultInfo
	}
	return &User{Name: userPass[0], PasswordMd5: StrMd5(userPass[1])}
}

const defaultTestUserName = "_test_"

var defaultTestUser = &User{
	Name:        defaultTestUserName,
	Password:    strconv.FormatInt(serverStartTime.UnixNano(), 10),
	PasswordMd5: StrMd5(strconv.FormatInt(serverStartTime.UnixNano(), 10)),
}

type userConfig struct {
	Users []*User
}

func loadUsers(confPath string) (users map[string]*User, err error) {
	var uc *userConfig
	if err = fsconf.Parse(confPath, &uc); err != nil {
		return nil, err
	}

	users = make(map[string]*User, len(uc.Users))

	for i := 0; i < len(uc.Users); i++ {
		u := uc.Users[i]
		if len(u.Name) == 0 {
			continue
		}
		if _, has := users[u.Name]; has {
			log.Println("dup name in users:", u.Name)
			continue
		}
		if len(u.PasswordMd5) == 0 {
			u.PasswordMd5 = StrMd5(u.Password)
		}
		if len(u.PasswordMd5) == 0 {
			log.Println("ignore User=", u.Name, "with empty password")
			continue
		}
		users[u.Name] = u
	}
	return users, nil
}

func (man *ProxyManager) checkHTTPAuth(user *User) bool {
	switch man.config.AuthType {
	case AuthTypeNO:
		return true
	case AuthTypeBasic:
		u := man.getUser(user.Name)
		if u != nil {
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
