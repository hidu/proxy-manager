package manager

import (
	"github.com/Unknwon/goconfig"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	title           string
	notice          string
	port            int
	confDir         string
	configFile      string
	timeout         int
	reTry           int
	reTryMax        int
	aliveCheckUrl   string
	checkInterval   int64
	authType        int
	wrongStatusCode map[int]int
}

const (
	AuthType_NO            = 0
	AuthType_Basic         = 1
	AuthType_Basic_WithAny = 2
)

func LoadConfig(configPath string) *Config {
	config := &Config{}
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

	wrongStatusCodeSlice := strings.Split(gconf.MustValue(goconfig.DEFAULT_SECTION, "wrongStatusCode", ""),",")

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

	aliveCheckUrl := gconf.MustValue(goconfig.DEFAULT_SECTION, "aliveCheck", "")
	_, err = url.Parse(aliveCheckUrl)
	if err != nil {
		log.Println("alive check url wrong:", err)
		return nil
	} else {
		config.aliveCheckUrl = aliveCheckUrl
	}

	return config
}
