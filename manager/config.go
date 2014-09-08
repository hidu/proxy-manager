package manager

import (
	"github.com/Unknwon/goconfig"
	"log"
	"os"
	"path/filepath"
)

type Config struct {
	port       int
	confDir    string
	configFile string
	timeout    int
	re_try     int
}

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
	config.port = gconf.MustInt(goconfig.DEFAULT_SECTION, "port", 8090)

	config.timeout = gconf.MustInt(goconfig.DEFAULT_SECTION, "timeout", 30)
	if config.timeout > 120 {
		config.timeout = 120
	}

	config.re_try = gconf.MustInt(goconfig.DEFAULT_SECTION, "re_try", 0)
	if config.re_try > 10 {
		config.re_try = 3
	}

	return config
}
