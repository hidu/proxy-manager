package internal

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hidu/goutils/fs"
	"github.com/hidu/goutils/net_util"
	"github.com/hidu/goutils/str_util"
	"github.com/hidu/goutils/time_util"
)

// ProxyPool 代理池
type ProxyPool struct {
	proxyListActive map[string]*Proxy
	proxyListAll    map[string]*Proxy
	mu              sync.RWMutex

	SessionProxys map[int64]map[string]*Proxy
	ProxyManager  *ProxyManager

	aliveCheckURL      string
	aliveCheckResponse *http.Response

	checkChan     chan string
	testRunChan   chan bool
	timeout       int
	checkInterval int64

	proxyUsed map[string]map[string]int64

	Count *proxyCount
}

// loadProxyPool 从配置文件中加载代理池
func loadProxyPool(manager *ProxyManager) *ProxyPool {
	log.Println("loading proxy pool...")
	pool := &ProxyPool{}
	pool.ProxyManager = manager
	pool.proxyListActive = make(map[string]*Proxy)
	pool.proxyListAll = make(map[string]*Proxy)
	pool.SessionProxys = make(map[int64]map[string]*Proxy)

	pool.proxyUsed = make(map[string]map[string]int64)

	pool.checkChan = make(chan string, 100)
	pool.testRunChan = make(chan bool, 1)
	pool.timeout = manager.config.timeout

	pool.aliveCheckURL = manager.config.aliveCheckURL
	pool.checkInterval = manager.config.checkInterval
	pool.Count = newProxyCount()

	if pool.aliveCheckURL != "" {
		var err error
		urlStr := strings.Replace(pool.aliveCheckURL, "{%rand}", fmt.Sprintf("%d", time.Now().UnixNano()), -1)
		pool.aliveCheckResponse, err = doRequestGet(urlStr, nil, pool.timeout)
		if err != nil {
			log.Println("get origin alive response failed,url:", pool.aliveCheckURL, "err:", err)
			return nil
		}
		log.Println("get alive info suc!url:", pool.aliveCheckURL, "resp_header:", pool.aliveCheckResponse.Header)
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

	time_util.SetInterval(func() {
		pool.runTest()
	}, pool.checkInterval)

	time_util.SetInterval(func() {
		pool.cleanProxyUsed()
	}, 1200)

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

	txtFile, err := str_util.NewTxtFile(confPath)
	if err != nil {
		log.Println("load proxy pool failed[", confName, "]")
		return proxys, err
	}
	return pool.loadProxysFromTxtFile(txtFile)
}

func (pool *ProxyPool) loadProxysFromTxtFile(txtFile *str_util.TxtFile) (map[string]*Proxy, error) {
	proxys := make(map[string]*Proxy)
	defaultValues := make(map[string]string)
	defaultValues["proxy"] = "required"
	defaultValues["weight"] = "1"
	defaultValues["status"] = "1"
	defaultValues["last_check"] = "0"
	defaultValues["check_used"] = "0"
	defaultValues["last_check_ok"] = "0"

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
	proxy := newProxy(info["proxy"])
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
	proxy.StatusCode = proxyStatus(intValues["status"])
	proxy.CheckUsed = int64(intValues["check_used"])
	proxy.LastCheck = int64(intValues["last_check"])
	proxy.LastCheckOk = int64(intValues["last_check_ok"])
	return proxy
}

func (pool *ProxyPool) getProxy(proxyURL string) *Proxy {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	if proxy, has := pool.proxyListAll[proxyURL]; has {
		return proxy
	}
	return nil
}

func (pool *ProxyPool) addProxyActive(proxyURL string) bool {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	if proxy, has := pool.proxyListAll[proxyURL]; has {
		if _, hasAct := pool.proxyListActive[proxyURL]; !hasAct {
			pool.proxyListActive[proxyURL] = proxy
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

func (pool *ProxyPool) removeProxyActive(proxyURL string) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if _, hasAct := pool.proxyListActive[proxyURL]; hasAct {
		delete(pool.proxyListActive, proxyURL)
	}
}

func (pool *ProxyPool) removeProxy(proxyURL string) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if _, hasAct := pool.proxyListAll[proxyURL]; hasAct {
		delete(pool.proxyListAll, proxyURL)
	}
	if _, hasAct := pool.proxyListActive[proxyURL]; hasAct {
		delete(pool.proxyListActive, proxyURL)
	}
}

var errorNoProxy = fmt.Errorf("no active proxy")

//
func (pool *ProxyPool) getOneProxy(uname string) (*Proxy, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	l := len(pool.proxyListActive)
	if l == 0 {
		return nil, errorNoProxy
	}

	userUsed, has := pool.proxyUsed[uname]
	if !has || len(userUsed) >= len(pool.proxyListActive) {
		userUsed = make(map[string]int64)
	}
	pool.proxyUsed[uname] = userUsed

	for _, proxy := range pool.proxyListActive {
		if _, has := userUsed[proxy.proxy]; has {
			continue
		}
		proxy.Used++
		userUsed[proxy.proxy] = time.Now().Unix()
		return proxy, nil
	}
	return nil, errorNoProxy
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
			pool.testProxyAddActive(proxyUrl)
			wg.Done()
		})(name)
	}
	wg.Wait()

	used := time.Now().Sub(start)
	log.Println("test all proxy finish,total:", proxyTotal, "used:", used, "activeTotal:", len(pool.proxyListActive))

	pool.cleanBadProxy(86400)

	testResultFile := pool.ProxyManager.config.confDir + "/pool_checked.conf"
	fs.FilePutContents(testResultFile, []byte(pool.String()))
}

// testProxyAddActive 测试一个代理是否可用 若可用则加入代理池否则删除
func (pool *ProxyPool) testProxyAddActive(proxyURL string) bool {
	proxy := pool.getProxy(proxyURL)
	if proxy == nil {
		return false
	}
	isOk := pool.testProxy(proxy)
	if isOk {
		pool.addProxyActive(proxy.proxy)
	} else {
		pool.removeProxyActive(proxy.proxy)
	}
	return true
}

func (pool *ProxyPool) testProxy(proxy *Proxy) bool {
	pool.checkChan <- proxy.proxy
	start := time.Now()
	defer (func() {
		<-pool.checkChan
	})()

	if start.Unix()-proxy.LastCheck < pool.checkInterval {
		return proxy.IsOk()
	}

	proxy.StatusCode = proxyStatusUnavaliable

	testlog := func(msg ...interface{}) {
		used := time.Now().Sub(start)
		proxy.CheckUsed = used.Nanoseconds() / 1000000
		proxy.LastCheck = start.Unix()
		log.Println("test proxy", proxy.proxy, fmt.Sprint(msg...), "used:", proxy.CheckUsed, "ms")
	}

	if pool.aliveCheckURL != "" {
		urlStr := strings.Replace(pool.aliveCheckURL, "{%rand}", fmt.Sprintf("%d", start.UnixNano()), -1)
		resp, err := doRequestGet(urlStr, proxy, pool.timeout)
		if err != nil {
			testlog("failed,", err.Error())
			return false
		}
		curLen := resp.Header.Get("Content-Length")
		checkLen := pool.aliveCheckResponse.Header.Get("Content-Length")
		if curLen != checkLen {
			testlog("failed ,content-length wrong,[", checkLen, "!=", curLen, "]")
			return false
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
	proxy.StatusCode = proxyStatusActive
	proxy.LastCheckOk = time.Now().Unix()
	testlog("pass")
	return true
}

// markProxyStatus 标记一个代理的连接状态
func (pool *ProxyPool) markProxyStatus(proxy *Proxy, status proxyUsedStatus) {
	proxy.Count.MarkStatus(status)
	pool.Count.MarkStatus(status)
}

// GetProxyNums 返回各种代理的数量 web页面会使用
func (pool *ProxyPool) GetProxyNums() NumsCount {
	data := newNumsCount()
	data.Add("total", len(pool.proxyListAll))
	data.Add("active", len(pool.proxyListActive))
	for _type := range proxyTransports {
		name := fmt.Sprintf("active_%s", _type)
		data.Add(name, 0)
	}
	for _, proxy := range pool.proxyListActive {

		name := fmt.Sprintf("active_%s", proxy.URL.Scheme)
		data.Add(name, 1)
	}
	return data
}

func doRequestGet(urlStr string, proxy *Proxy, timeoutSec int) (resp *http.Response, err error) {
	client := &http.Client{}
	if proxy != nil {
		client, err = newClient(proxy.URL, timeoutSec)
		if err != nil {
			return nil, err
		}
	}
	if timeoutSec > 0 {
		client.Timeout = time.Duration(timeoutSec) * time.Second
	}
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func (pool *ProxyPool) cleanBadProxy(sec int64) {
	last := time.Now().Unix() - sec
	var proxyBad []*Proxy
	for _, proxy := range pool.proxyListAll {
		if proxy.LastCheckOk < last {
			proxyBad = append(proxyBad, proxy)
		}
	}

	for _, proxy := range proxyBad {
		pool.removeProxy(proxy.proxy)
		fs.FilePutContents(pool.ProxyManager.config.confDir+"/pool_bad.list", []byte(proxy.String()+"\n"), fs.FILE_APPEND)
	}
}

func (pool *ProxyPool) cleanProxyUsed() {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	now := time.Now().Unix()
	delNames := []string{}
	for name, infos := range pool.proxyUsed {
		delKeys := []string{}
		for k, v := range infos {
			if now-v > 1200 {
				delKeys = append(delKeys, k)
			}
		}
		for _, k := range delKeys {
			delete(infos, k)
		}
		if len(infos) == 0 {
			delNames = append(delNames, name)
		}
	}

	for _, name := range delNames {
		delete(pool.proxyUsed, name)
		log.Println("cleanProxyUsed name=", name)
	}
}
