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

	return config
}
