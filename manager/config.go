package manager

import (

)

type Config struct{
  port int
  confDir string
}

func NewConfig() *Config{
  config:=&Config{}
  config.port=8090
  return config
}