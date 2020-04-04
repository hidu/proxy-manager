package internal

import (
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Unknwon/goconfig"
)

type config struct {
	title           string
	notice          string
	port            int
	confDir         string
	configFile      string
	timeout         int
	reTry           int
	reTryMax        int
	aliveCheckURL   string
	checkInterval   int64
	authType        int
	wrongStatusCode map[int]int
}

const (
	// AuthTypeNO 不需要认证
	AuthTypeNO = 0
	// AuthTypeBasic 使用basic
	AuthTypeBasic = 1
	// AuthTypeBasicWithAny 使用basic，任意账号密码都可以
	AuthTypeBasicWithAny = 2
)

func loadConfig(configPath string) *config {
	config := &config{}
	absPath, err := filepath.Abs(configPath)

	config.configFile = absPath
	if err != nil {
		log.Println("get config path failed", configPath)
		return nil
	}

	config.confDir = filepath.Dir(absPath)

	os.Chdir(config.confDir)

	gconf, err := goconfig.LoadConfigFile(config.configFile)
	if err != nil {
		log.Println("load config failed:", err)
		return nil
	}
	config.title = gconf.MustValue(goconfig.DEFAULT_SECTION, "title", "")

	config.notice = gconf.MustValue(goconfig.DEFAULT_SECTION, "notice", "")

	config.wrongStatusCode = make(map[int]int)

	wrongStatusCodeSlice := strings.Split(gconf.MustValue(goconfig.DEFAULT_SECTION, "wrongStatusCode", ""), ",")

	for _, v := range wrongStatusCodeSlice {
		_code := int(getInt64(strings.TrimSpace(v)))
		if _code > 0 {
			config.wrongStatusCode[_code] = _code
		}
	}

	config.port = gconf.MustInt(goconfig.DEFAULT_SECTION, "port", 8090)

	config.timeout = gconf.MustInt(goconfig.DEFAULT_SECTION, "timeout", 30)
	if config.timeout > 120 {
		config.timeout = 120
	}
	config.checkInterval = gconf.MustInt64(goconfig.DEFAULT_SECTION, "checkInterval", 3600)
	if config.checkInterval <= 60 {
		config.checkInterval = 1800
	}

	_authType := strings.ToLower(gconf.MustValue(goconfig.DEFAULT_SECTION, "authType", "none"))
	authTypes := map[string]int{"none": 0, "basic": 1, "basic_any": 2}

	if authType, has := authTypes[_authType]; has {
		config.authType = authType
	} else {
		log.Println("conf error,unknow value authType:", _authType)
	}

	config.reTry = gconf.MustInt(goconfig.DEFAULT_SECTION, "reTry", 0)

	config.reTryMax = gconf.MustInt(goconfig.DEFAULT_SECTION, "reTryMax", 0)

	aliveCheckURL := gconf.MustValue(goconfig.DEFAULT_SECTION, "aliveCheck", "")
	_, err = url.Parse(aliveCheckURL)
	if err != nil {
		log.Println("alive check url wrong:", err)
		return nil
	}
	config.aliveCheckURL = aliveCheckURL

	return config
}

// InitConf 第一次运行 初始化配置文件目录
func InitConf(confDir string) {
	stat, err := os.Stat(confDir)
	if err == nil {
		err = os.Chdir(confDir)
	}
	if err != nil {
		log.Println("err:", err)
		os.Exit(1)
	}
	if !stat.IsDir() {
		log.Println("not dir")
		os.Exit(1)
	}
	stat, err = os.Stat("proxy.conf")

	if os.IsExist(err) {
		log.Println("proxy.conf exists!")
		os.Exit(1)
	}

	ioutil.WriteFile("proxy.conf", Asset.GetContent("/res/conf/proxy.conf"), 0644)
	ioutil.WriteFile("pool.conf", Asset.GetContent("/res/conf/pool.conf"), 0644)
	ioutil.WriteFile("users", Asset.GetContent("/res/conf/users"), 0644)
	log.Println("init conf done")
}
