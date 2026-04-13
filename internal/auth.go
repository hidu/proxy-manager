package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/xanygo/anygo/ds/xsync"
	"github.com/xanygo/anygo/xattr"
	"github.com/xanygo/anygo/xcfg"
)

const proxyAuthorizationHeader = "Proxy-Authorization"

type User struct {
	Name        string `yaml:"Name"`
	Password    string `yaml:"Password"`
	PasswordMd5 string `yaml:"PasswordMd5"`
	Admin       bool   `yaml:"Admin"`
}

func (u *User) pswEq(psw string) bool {
	return u.PasswordMd5 == StrMd5(psw)
}

func (u *User) PswEnc() string {
	return StrMd5(fmt.Sprintf("%s:%s", u.Name, u.PasswordMd5))
}

func (u *User) Equal(u1 *User) bool {
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

type userConfig struct {
	Users []*User `yaml:"Users"`
}

var usersStore = &xsync.OnceInit[map[string]*User]{
	New: func() map[string]*User {
		users, err := loadUsers("users.yml")
		if err != nil {
			log.Fatalln("loadUsers err:", err)
		}
		log.Println("loadUsers success, total:", len(users))
		return users
	},
}

func getUser(name string) *User {
	users := usersStore.Load()
	if len(users) == 0 {
		return nil
	}
	return users[name]
}

func loadUsers(confPath string) (users map[string]*User, err error) {
	var uc *userConfig
	if err = xcfg.Parse(confPath, &uc); err != nil {
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
		if len(u.PasswordMd5) == 0 && u.Password != "" {
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

func checkHTTPAuth(user *User) bool {
	authType := xattr.GetDefault[string]("AuthType", AuthTypeNO)
	switch authType {
	case AuthTypeNO:
		return true
	case AuthTypeBasic:
		u := getUser(user.Name)
		if u != nil {
			return u.Equal(user)
		}
		return false
	case AuthTypeBasicWithAny:
		return user.Name != ""
	default:
		return false
	}
}

const (
	// AuthTypeNO 不需要认证
	AuthTypeNO = "no"

	// AuthTypeBasic 使用basic
	AuthTypeBasic = "basic"

	// AuthTypeBasicWithAny 使用 basic，任意账号密码都可以
	AuthTypeBasicWithAny = "basic_any"
)
