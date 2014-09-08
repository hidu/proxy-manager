package manager

import (
	"fmt"
	"github.com/hidu/goutils"
	"log"
	//	"math/rand"
	"net/url"
	//	"strings"
	"strconv"
	"sync"
)

type Proxy struct {
	proxy  string
	URL    *url.URL
	Weight int
}

func NewProxy(proxyUrl string) *Proxy {
	proxy := &Proxy{proxy: proxyUrl}
	var err error
	proxy.URL, err = url.Parse(proxyUrl)
	if err != nil {
		log.Println("proxy info wrong", err)
		return nil
	}
	proxy.Weight = 100
	return proxy
}

var ProxyNo = NewProxy("htp://127.0.0.1:12356")

type ProxyPool struct {
	proxyListActive map[string]*Proxy
	proxyListAll    map[string]*Proxy
	mu              sync.RWMutex

	proxyUsed map[int64]map[string]*Proxy
}

func LoadProxyPool(confDir string) *ProxyPool {
	pool := &ProxyPool{}
	pool.proxyListActive = make(map[string]*Proxy)
	pool.proxyListAll = make(map[string]*Proxy)
	pool.proxyUsed = make(map[int64]map[string]*Proxy)

	confPath := confDir + "/pool.conf"

	txtFile, err := utils.NewTxtFile(confPath)
	if err != nil {
		log.Println("load proxy pool failed")
		return nil
	}
	defaultValues := make(map[string]string)
	defaultValues["proxy"] = "required"
	defaultValues["weight"] = "1"

	datas, err := txtFile.KvMapSlice("=", true, defaultValues)
	for _, kv := range datas {
		pool.addProxy(kv)
		pool.addProxyActive(kv["proxy"])
	}
	return pool
}

func (pool *ProxyPool) addProxy(info map[string]string) {
	if info == nil {
		return
	}
	proxy := NewProxy(info["proxy"])
	if proxy == nil {
		return
	}
	var err error
	proxy.Weight, err = strconv.Atoi(info["weight"])
	if err != nil {
		proxy.Weight = 100
	}
	if _, has := pool.proxyListAll[proxy.proxy]; !has {
		pool.proxyListAll[proxy.proxy] = proxy
	}
}

func (pool *ProxyPool) addProxyActive(proxy_url string) bool {
	if proxy, has := pool.proxyListAll[proxy_url]; has {
		if _, hasAct := pool.proxyListActive[proxy_url]; !hasAct {
			pool.proxyListActive[proxy_url] = proxy
			return true
		}
	}
	return false
}

var errorNoProxy error = fmt.Errorf("no active proxy")

func (pool *ProxyPool) GetOneProxy(logid int64) (*Proxy, error) {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	l := len(pool.proxyListActive)
	if l == 0 {
		return nil, errorNoProxy
	}

	sessionProxys, has := pool.proxyUsed[logid]

	if !has {
		sessionProxys = make(map[string]*Proxy)
		pool.proxyUsed[logid] = sessionProxys
	}

	for _, proxy := range pool.proxyListActive {
		if _, has := sessionProxys[proxy.proxy]; !has {
			sessionProxys[proxy.proxy] = proxy
			return proxy, nil
		}
	}
	return nil, errorNoProxy
}

func (pool *ProxyPool) CleanSessionProxy(logid int64) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if _, has := pool.proxyUsed[logid]; has {
		delete(pool.proxyUsed, logid)
	}
}

//func (pool *ProxyPool) GetOnePeoxyUrl(req *http.Request) (*url.URL, error) {
//	proxy, err := pool.GetOneProxy()
//	if err != nil {
//		log.Println(err)
//		return ProxyNo.urlObj, err
//	}
//	return proxy.urlObj, nil
//}
