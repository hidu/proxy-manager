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
	"strings"
	"sync"
	"time"
)

type ProxyPool struct {
	proxyListActive map[string]*Proxy
	proxyListAll    map[string]*Proxy
	mu              sync.RWMutex

	SessionProxys map[int64]map[string]*Proxy
	ProxyManager  *ProxyManager

	aliveCheckUrl      string
	aliveCheckResponse *http.Response

	checkChan     chan string
	testRunChan   chan bool
	timeout       int
	checkInterval int64

	proxyActiveUsed map[string]string

	Count *ProxyCount
}

func LoadProxyPool(manager *ProxyManager) *ProxyPool {
	log.Println("loading proxy pool...")
	pool := &ProxyPool{}
	pool.ProxyManager = manager
	pool.proxyListActive = make(map[string]*Proxy)
	pool.proxyListAll = make(map[string]*Proxy)
	pool.SessionProxys = make(map[int64]map[string]*Proxy)

	pool.proxyActiveUsed = make(map[string]string)

	pool.checkChan = make(chan string, 100)
	pool.testRunChan = make(chan bool, 1)
	pool.timeout = manager.config.timeout

	pool.aliveCheckUrl = manager.config.aliveCheckUrl
	pool.checkInterval = manager.config.checkInterval
	pool.Count = NewProxyCount()

	if pool.aliveCheckUrl != "" {
		var err error
		urlStr := strings.Replace(pool.aliveCheckUrl, "{%rand}", fmt.Sprintf("%d", time.Now().UnixNano()), -1)
		pool.aliveCheckResponse, err = doRequestGet(urlStr, nil, 3)
		if err != nil {
			log.Println("get origin alive response failed,url:", pool.aliveCheckUrl, "err:", err)
			return nil
		} else {
			log.Println("get alive info suc!url:", pool.aliveCheckUrl, "resp_header:", pool.aliveCheckResponse.Header)
		}
	}

	proxyAll, err := pool.loadConf("pool.conf")
	if err != nil {
		log.Println("pool.conf not exists")
	}
	proxyAllChecked, _ := pool.loadConf("pool_checked.conf")

	pool.proxyListAll = proxyAllChecked
	for _url, proxy := range proxyAll {
		if _, has := pool.proxyListAll[_url]; !has {
			pool.proxyListAll[_url] = proxy
		}
	}
	if len(pool.proxyListAll) == 0 {
		log.Println("proxy pool list is empty")
	}

	go pool.runTest()

	utils.SetInterval(func() {
		pool.runTest()
	}, pool.checkInterval)

	return pool
}

func (pool *ProxyPool) String() string {
	allProxy := []string{}
	for _, proxy := range pool.proxyListAll {
		allProxy = append(allProxy, proxy.String())
	}
	return strings.Join(allProxy, "\n")
}

func (pool *ProxyPool) loadConf(confName string) (map[string]*Proxy, error) {
	proxys := make(map[string]*Proxy)
	confPath := pool.ProxyManager.config.confDir + "/" + confName

	txtFile, err := utils.NewTxtFile(confPath)
	if err != nil {
		log.Println("load proxy pool failed[", confName, "]")
		return proxys, err
	}
	return pool.loadProxysFromTxtFile(txtFile)
}

func (pool *ProxyPool) loadProxysFromTxtFile(txtFile *utils.TxtFile) (map[string]*Proxy, error) {
	proxys := make(map[string]*Proxy)
	defaultValues := make(map[string]string)
	defaultValues["proxy"] = "required"
	defaultValues["weight"] = "1"
	defaultValues["status"] = "1"
	defaultValues["last_check"] = "0"
	defaultValues["check_used"] = "0"

	datas, err := txtFile.KvMapSlice("=", true, defaultValues)
	if err != nil {
		return proxys, err
	}
	for _, kv := range datas {
		proxy := pool.parseProxy(kv)
		if proxy != nil {
			proxys[proxy.proxy] = proxy
		}
	}
	return proxys, nil
}

func (pool *ProxyPool) parseProxy(info map[string]string) *Proxy {
	if info == nil {
		return nil
	}
	proxy := NewProxy(info["proxy"])
	if proxy == nil {
		return nil
	}
	intValues := make(map[string]int)
	intFields := []string{"weight", "status", "check_used", "last_check", "last_check_ok"}
	var err error
	for _, fieldName := range intFields {
		intValues[fieldName], err = strconv.Atoi(info[fieldName])
		if err != nil {
			log.Println("parse [", fieldName, "]failed,not int.err:", err)
			intValues[fieldName] = 0
		}
	}
	proxy.Weight = intValues["weight"]
	proxy.StatusCode = PROXY_STATUS(intValues["status"])
	proxy.CheckUsed = int64(intValues["check_used"])
	proxy.LastCheck = int64(intValues["last_check"])
	proxy.LastCheckOk = int64(intValues["last_check_ok"])
	return proxy
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

func (pool *ProxyPool) addProxy(proxy *Proxy) bool {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	if _, has := pool.proxyListAll[proxy.proxy]; !has {
		pool.proxyListAll[proxy.proxy] = proxy
		return true
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
	pool.mu.Lock()
	defer pool.mu.Unlock()
	l := len(pool.proxyListActive)
	if l == 0 {
		return nil, errorNoProxy
	}

	sessionProxys, has := pool.SessionProxys[logid]

	if !has {
		sessionProxys = make(map[string]*Proxy)
		pool.SessionProxys[logid] = sessionProxys
	}

	for _, proxy := range pool.proxyListActive {
		if _, has := pool.proxyActiveUsed[proxy.proxy]; has {
			continue
		}
		if _, has := sessionProxys[proxy.proxy]; !has {
			sessionProxys[proxy.proxy] = proxy
			proxy.Used++
			pool.proxyActiveUsed[proxy.proxy] = "1"
			if len(pool.proxyActiveUsed) >= len(pool.proxyListActive) {
				pool.proxyActiveUsed = make(map[string]string)
			}
			return proxy, nil
		}
	}
	return nil, errorNoProxy
}

func (pool *ProxyPool) CleanSessionProxy(logid int64) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if _, has := pool.SessionProxys[logid]; has {
		delete(pool.SessionProxys, logid)
	}
}
func (pool *ProxyPool) runTest() {
	pool.testRunChan <- true
	defer (func() {
		<-pool.testRunChan
	})()
	start := time.Now()
	proxyTotal := len(pool.proxyListAll)
	log.Println("start test all proxy,total=", proxyTotal)

	var wg sync.WaitGroup
	for name := range pool.proxyListAll {
		wg.Add(1)
		go (func(proxyUrl string) {
			pool.TestProxyAddActive(proxyUrl)
			wg.Done()
		})(name)
	}
	wg.Wait()

	used := time.Now().Sub(start)
	log.Println("test all proxy finish,total:", proxyTotal, "used:", used, "activeTotal:", len(pool.proxyListActive))

	testResultFile := pool.ProxyManager.config.confDir + "/pool_checked.conf"
	utils.File_put_contents(testResultFile, []byte(pool.String()))
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
	pool.checkChan <- proxy.proxy
	start := time.Now()
	defer (func() {
		<-pool.checkChan
	})()

	if start.Unix()-proxy.LastCheck < pool.checkInterval {
		return proxy.IsOk()
	}

	proxy.StatusCode = PROXY_STATUS_UNAVAILABLE

	testlog := func(msg ...interface{}) {
		used := time.Now().Sub(start)
		proxy.CheckUsed = used.Nanoseconds() / 1000000
		proxy.LastCheck = start.Unix()
		log.Println("test proxy", proxy.proxy, fmt.Sprint(msg...), "used:", proxy.CheckUsed, "ms")
	}

	if pool.aliveCheckUrl != "" {
		urlStr := strings.Replace(pool.aliveCheckUrl, "{%rand}", fmt.Sprintf("%d", start.UnixNano()), -1)
		resp, err := doRequestGet(urlStr, proxy, pool.timeout/2)
		if err != nil {
			testlog("failed,", err.Error())
			return false
		} else {
			cur_len := resp.Header.Get("Content-Length")
			check_len := pool.aliveCheckResponse.Header.Get("Content-Length")
			if cur_len != check_len {
				testlog("failed ,content-length wrong,[", check_len, "!=", cur_len, "]")
				return false
			}
		}
	} else {
		host, port, err := utils.Net_getHostPortFromUrl(proxy.proxy)
		if err != nil {
			testlog("failed,proxy url err:", err)
			return false
		}
		conn, netErr := net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
		if netErr != nil {
			testlog("failed", netErr)
			return false
		}
		conn.Close()
	}
	proxy.StatusCode = PROXY_STATUS_ACTIVE
	proxy.LastCheckOk = proxy.LastCheck
	testlog("pass")
	return true
}

func (pool *ProxyPool) MarkProxyStatus(proxy *Proxy, status PROXY_USED_STATUS) {
	proxy.Count.MarkStatus(status)
	pool.Count.MarkStatus(status)
}

func doRequestGet(urlStr string, proxy *Proxy, timeout_sec int) (resp *http.Response, err error) {
	client := &http.Client{}
	if timeout_sec > 0 {
		client.Timeout = time.Duration(timeout_sec) * time.Second
	}
	if proxy != nil {
		proxyGetFn := func(req *http.Request) (*url.URL, error) {
			return proxy.URL, nil
		}
		client.Transport = &http.Transport{Proxy: proxyGetFn}
	}
	req, _ := http.NewRequest("GET", urlStr, nil)
	return client.Do(req)
}
