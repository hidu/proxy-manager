package manager

import (
  "fmt"
  "sync"
  "math/rand"
)
type Proxy struct{
	url string
}

type ProxyPool struct{
	proxyListActive []*Proxy
	proxyListAll []*Proxy
	mu sync.RWMutex
	confDir string
}

func LoadProxyPool(confDir string) *ProxyPool{
   pool:=&ProxyPool{}
   pool.confDir=confDir
   return pool
}

func (pool *ProxyPool)GetOneProxy() (*Proxy,error){
  pool.mu.RLock()
  defer pool.mu.RUnlock()
  l:=len(pool.proxyListActive)
  if(l==0){
   	 return nil,fmt.Errorf("no active proxy")
  }
  
  return pool.proxyListActive[rand.Int()%l],nil
}
