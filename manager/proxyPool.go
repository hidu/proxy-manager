package manager

import (
	"fmt"
	"github.com/hidu/goutils"
	"log"
	//	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
)

type Proxy struct {
	proxy      string
	URL        *url.URL
	Weight     int
	StatusCode int
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

type ProxyPool struct {
	proxyListActive map[string]*Proxy
	proxyListAll    map[string]*Proxy
	mu              sync.RWMutex

	proxyUsed          map[int64]map[string]*Proxy
	ProxyManager       *ProxyManager
	aliveCheckUrl      string
	aliveCheckResponse *http.Response
}

func LoadProxyPool(manager *ProxyManager) *ProxyPool {
	pool := &ProxyPool{}
	pool.ProxyManager = manager
	pool.proxyListActive = make(map[string]*Proxy)
	pool.proxyListAll = make(map[string]*Proxy)
	pool.proxyUsed = make(map[int64]map[string]*Proxy)

	pool.aliveCheckUrl = manager.config.aliveCheckUrl

	if pool.aliveCheckUrl != "" {
		var err error
		pool.aliveCheckResponse, err = doRequestGet(pool.aliveCheckUrl, nil)
		if err != nil {
			log.Println("get origin alive response failed,url:", pool.aliveCheckUrl, "err:", err)
			return nil
		} else {
			log.Println("get alive info suc!url:", pool.aliveCheckUrl, "resp_header:", pool.aliveCheckResponse.Header)
		}
	}

	pool.loadConf("pool.conf")
	return pool
}

func (pool *ProxyPool) loadConf(confName string) int {
	confPath := pool.ProxyManager.config.confDir + "/" + confName

	txtFile, err := utils.NewTxtFile(confPath)
	if err != nil {
		log.Println("load proxy pool failed")
		return 0
	}
	defaultValues := make(map[string]string)
	defaultValues["proxy"] = "required"
	defaultValues["weight"] = "1"
	defaultValues["status"] = "1"

	datas, err := txtFile.KvMapSlice("=", true, defaultValues)
	for _, kv := range datas {
		pool.addProxy(kv)
		go pool.TestProxyAddActive(kv["proxy"])
	}
	log.Println(len(datas), "proxy in pool")
	return len(datas)
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
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if _, has := pool.proxyListAll[proxy.proxy]; !has {
		pool.proxyListAll[proxy.proxy] = proxy
	}
}

func (pool *ProxyPool) GetProxy(proxy_url string) *Proxy {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	if proxy, has := pool.proxyListAll[proxy_url]; has {
		return proxy
	}
	return nil
}

func (pool *ProxyPool) addProxyActive(proxy_url string) bool {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	if proxy, has := pool.proxyListAll[proxy_url]; has {
		if _, hasAct := pool.proxyListActive[proxy_url]; !hasAct {
			pool.proxyListActive[proxy_url] = proxy
			return true
		}
	}
	return false
}

func (pool *ProxyPool) removeProxyActive(proxy_url string) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if _, hasAct := pool.proxyListActive[proxy_url]; hasAct {
		delete(pool.proxyListActive, proxy_url)
	}
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
func (pool *ProxyPool) TestProxyAddActive(proxy_url string) bool {
	proxy := pool.GetProxy(proxy_url)
	if proxy == nil {
		return false
	}
	isOk := pool.TestProxy(proxy)
	if isOk {
		pool.addProxyActive(proxy.proxy)
	} else {
		pool.removeProxyActive(proxy.proxy)
	}
	return true
}

func (pool *ProxyPool) TestProxy(proxy *Proxy) bool {
	if pool.aliveCheckUrl != "" {
		resp, err := doRequestGet(pool.aliveCheckUrl, proxy)
		if err != nil {
			log.Println("test proxy failed:[", proxy.proxy, "]", err)
			return false
		} else {
			cur_len := resp.Header.Get("Content-Length")
			check_len := pool.aliveCheckResponse.Header.Get("Content-Length")
			if cur_len != check_len {
				log.Println("test proxy failed:[", proxy.proxy, "],Content-Length wrong,[", check_len, "!=", cur_len, "]")
				return false
			}
		}
	} else {
		host, port, err := utils.Net_getHostPortFromUrl(proxy.proxy)
		if err != nil {
			log.Println("proxy url err:", err)
			return false
		}
		conn, netErr := net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
		if netErr != nil {
			log.Println("test proxy failed:[", proxy.proxy, "]", netErr)
			return false
		}
		conn.Close()
	}
	log.Println("test proxy pass,[", proxy.proxy, "]")
	return true
}

func doRequestGet(urlStr string, proxy *Proxy) (resp *http.Response, err error) {
	client := &http.Client{}
	if proxy != nil {
		proxyGetFn := func(req *http.Request) (*url.URL, error) {
			return proxy.URL, nil
		}
		client.Transport = &http.Transport{Proxy: proxyGetFn}
	}

	req, _ := http.NewRequest("GET", urlStr, nil)
	return client.Do(req)
}
