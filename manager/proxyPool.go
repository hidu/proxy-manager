package manager

import (
	"fmt"
	"github.com/hidu/goutils"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type Proxy struct {
	proxyUrl string
	urlObj   *url.URL
}

func NewProxy(proxyUrl string) *Proxy {
	proxy := &Proxy{proxyUrl: proxyUrl}
	var err error
	proxy.urlObj, err = url.Parse(proxyUrl)
	if err != nil {
		log.Println("proxy info wrong", err)
		return nil
	}
	return proxy
}

var ProxyNo = NewProxy("htp://127.0.0.1:12356")

type ProxyPool struct {
	proxyListActive map[string]*Proxy
	proxyListAll    map[string]*Proxy
	mu              sync.RWMutex
}

func LoadProxyPool(confDir string) *ProxyPool {
	pool := &ProxyPool{}
	pool.proxyListActive = make(map[string]*Proxy)
	pool.proxyListAll = make(map[string]*Proxy)

	confPath := confDir + "/pool.conf"

	txtFile, err := utils.NewTxtFile(confPath)
	if err != nil {
		log.Println("load proxy pool failed")
		return nil
	}
	for _, line := range txtFile.Lines {
		arr := line.Slice()
		if len(arr) > 0 {
			pool.addProxy(arr[0])
			pool.addProxyActive(arr[0])
		}
	}
	return pool
}

func (pool *ProxyPool) addProxy(proxy_url string) {
	proxy_url = strings.TrimSpace(proxy_url)
	if proxy_url == "" {
		return
	}
	proxy := NewProxy(proxy_url)
	if proxy != nil {
		if _, has := pool.proxyListAll[proxy.proxyUrl]; !has {
			pool.proxyListAll[proxy.proxyUrl] = proxy
		}
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

func (pool *ProxyPool) GetOneProxy() (*Proxy, error) {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	l := len(pool.proxyListActive)
	if l == 0 {
		return nil, fmt.Errorf("no active proxy")
	}
	n := rand.Int() % l
	index := 0
	for _, proxy := range pool.proxyListActive {
		if index == n {
			return proxy, nil
		}
		index++
	}
	return nil, fmt.Errorf("miss")
}

func (pool *ProxyPool) GetOnePeoxyUrl(req *http.Request) (*url.URL, error) {
	proxy, err := pool.GetOneProxy()
	if err != nil {
		log.Println(err)
		return ProxyNo.urlObj, err
	}
	return proxy.urlObj, nil
}
