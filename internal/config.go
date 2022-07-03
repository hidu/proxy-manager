package internal

import (
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fsgo/fsconf"
)

type Config struct {
	Title  string
	Notice string

	Port     int //  必填，服务端口
	Timeout  int // 可选，超时时间，单位秒,默认 30
	ReTry    int // 可选，重试次数，默认 2
	ReTryMax int // 可选，最大重试次数由客户端通过http header [X-Man-Retry]指定

	AliveCheckURL   string // 必填，通过检测这个url来判断代理是否正常
	CheckInterval   int    // 可选，检测代理有效的间隔时间,单位秒，默认 1800
	AuthType        string // 可选，鉴权类型，可选值 no-不需要鉴权，basic、basic_any-任意帐号
	WrongStatusCode []int
}

func (c *Config) IsWrongCode(code int) bool {
	for i := 0; i < len(c.WrongStatusCode); i++ {
		if c.WrongStatusCode[i] == code {
			return true
		}
	}
	return false
}

func (c *Config) getAliveCheckURL() string {
	str := strconv.FormatInt(time.Now().UnixNano(), 10)
	return strings.ReplaceAll(c.AliveCheckURL, "{%rand}", str)
}

func (c *Config) getCheckInterval() time.Duration {
	if c.CheckInterval > 0 {
		return time.Duration(c.CheckInterval) * time.Second
	}
	return 1800 * time.Second
}

func (c *Config) getTimeout() time.Duration {
	if c.Timeout > 0 {
		return time.Duration(c.Timeout) * time.Second
	}
	return 30 * time.Second
}

func (c *Config) getReTry() int {
	if c.ReTry > 0 {
		return c.ReTry
	}
	return 2
}

const (
	// AuthTypeNO 不需要认证
	AuthTypeNO = "no"

	// AuthTypeBasic 使用basic
	AuthTypeBasic = "basic"

	// AuthTypeBasicWithAny 使用basic，任意账号密码都可以
	AuthTypeBasicWithAny = "basic_any"
)

func loadConfig(fp string) (*Config, error) {
	var c *Config
	err := fsconf.Parse(fp, &c)
	return c, err
}

// InitConf 第一次运行 初始化配置文件目录
func InitConf(confDir string) {
	stat, err := os.Stat(confDir)
	if err == nil {
		err = os.Chdir(confDir)
	}
	if err != nil {
		log.Fatalln("err:", err)
	}

	if !stat.IsDir() {
		log.Fatalln(confDir, "is not dir")
	}

	stat, err = os.Stat("proxy.toml")

	if os.IsExist(err) {
		log.Fatalln("proxy.toml exists!")
	}

	ioutil.WriteFile("proxy.toml", AssetGetContent("conf/proxy.toml"), 0644)
	ioutil.WriteFile("pool.conf", AssetGetContent("conf/pool.conf"), 0644)
	ioutil.WriteFile("users.toml", AssetGetContent("conf/users.toml"), 0644)
	log.Println("init conf done")
}
