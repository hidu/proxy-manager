package internal

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsgo/fsenv"
	"github.com/hidu/goutils/fs"
	"github.com/hidu/goutils/str_util"
)

// ProxyPool 代理池
type ProxyPool struct {
	proxyListActive map[string]*Proxy
	proxyListAll    map[string]*Proxy
	mu              sync.RWMutex

	SessionProxys map[int64]map[string]*Proxy

	config *Config

	aliveCheckResponse *http.Response

	checkChan   chan string
	testRunChan chan bool

	proxyUsed map[string]map[string]int64

	Count *proxyCount
}

// loadPool 从配置文件中加载代理池
func loadPool(cfg *Config) *ProxyPool {
	log.Println("loading proxy pool...")
	pool := &ProxyPool{}
	pool.config = cfg
	pool.proxyListActive = make(map[string]*Proxy)
	pool.proxyListAll = make(map[string]*Proxy)
	pool.SessionProxys = make(map[int64]map[string]*Proxy)

	pool.proxyUsed = make(map[string]map[string]int64)

	pool.checkChan = make(chan string, 100)
	pool.testRunChan = make(chan bool, 1)

	pool.Count = newProxyCount()

	checkURL := pool.config.getAliveCheckURL()

	if len(checkURL) > 0 {
		var err error
		pool.aliveCheckResponse, err = doRequestGet(checkURL, nil, pool.config.getTimeout())
		if err != nil {
			log.Println("get alive response failed, url:", checkURL, "err:", err)
			return nil
		}
		log.Println("get alive info success! url:", checkURL, "resp_header:", pool.aliveCheckResponse.Header)
	}

	proxyAll, err := pool.loadConf("pool.conf")
	if err != nil {
		log.Println("pool.conf not exists")
	}
	proxyAllChecked, err := pool.loadConf("pool_checked.conf")
	if err != nil {
		log.Println("loadConf")
	}
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

	SetInterval(func() {
		pool.runTest()
	}, pool.config.getCheckInterval())

	SetInterval(func() {
		pool.cleanProxyUsed()
	}, 1200*time.Second)

	return pool
}

func (p *ProxyPool) String() string {
	var allProxy []string
	for _, proxy := range p.proxyListAll {
		allProxy = append(allProxy, proxy.String())
	}
	return strings.Join(allProxy, "\n")
}

func (p *ProxyPool) loadConf(confName string) (map[string]*Proxy, error) {
	proxies := make(map[string]*Proxy)
	confPath := filepath.Join(fsenv.ConfRootDir(), confName)
	txtFile, err := str_util.NewTxtFile(confPath)
	if err != nil {
		log.Println("load proxy p failed[", confName, "]")
		return proxies, err
	}
	return p.loadProxiesFromTxtFile(txtFile)
}

func (p *ProxyPool) loadProxiesFromTxtFile(txtFile *str_util.TxtFile) (map[string]*Proxy, error) {
	proxies := make(map[string]*Proxy)
	defaultValues := make(map[string]string)
	defaultValues["proxy"] = "required"
	defaultValues["weight"] = "1"
	defaultValues["status"] = "1"
	defaultValues["last_check"] = "0"
	defaultValues["check_used"] = "0"
	defaultValues["last_check_ok"] = "0"

	datas, err := txtFile.KvMapSlice("=", true, defaultValues)
	if err != nil {
		return proxies, err
	}
	for _, kv := range datas {
		proxy := p.parseProxy(kv)
		if proxy != nil {
			proxies[proxy.proxy] = proxy
		}
	}
	return proxies, nil
}

func (p *ProxyPool) parseProxy(info map[string]string) *Proxy {
	if info == nil {
		return nil
	}
	proxy := newProxy(info["proxy"])
	if proxy == nil {
		return nil
	}
	intValues := make(map[string]int)
	intFields := []string{"weight", "status", "last_check", "last_check_ok"}
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
	proxy.CheckUsed, _ = time.ParseDuration(info["check_used"])
	proxy.LastCheck = time.Unix(int64(intValues["last_check"]), 0)
	proxy.LastCheckOk = time.Unix(int64(intValues["last_check_ok"]), 0)
	return proxy
}

func (p *ProxyPool) getProxy(proxyURL string) *Proxy {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if proxy, has := p.proxyListAll[proxyURL]; has {
		return proxy
	}
	return nil
}

func (p *ProxyPool) addProxyActive(proxyURL string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if proxy, has := p.proxyListAll[proxyURL]; has {
		if _, hasAct := p.proxyListActive[proxyURL]; !hasAct {
			p.proxyListActive[proxyURL] = proxy
			return true
		}
	}
	return false
}

func (p *ProxyPool) addProxy(proxy *Proxy) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, has := p.proxyListAll[proxy.proxy]; !has {
		p.proxyListAll[proxy.proxy] = proxy
		return true
	}
	return false
}

func (p *ProxyPool) removeProxyActive(proxyURL string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, has := p.proxyListActive[proxyURL]; has {
		delete(p.proxyListActive, proxyURL)
	}
}

func (p *ProxyPool) removeProxy(proxyURL string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, has := p.proxyListAll[proxyURL]; has {
		delete(p.proxyListAll, proxyURL)
	}
	if _, has := p.proxyListActive[proxyURL]; has {
		delete(p.proxyListActive, proxyURL)
	}
}

var errorNoProxy = fmt.Errorf("no active proxy")

//
func (p *ProxyPool) getOneProxy(uname string) (*Proxy, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	l := len(p.proxyListActive)
	if l == 0 {
		return nil, errorNoProxy
	}

	userUsed, has := p.proxyUsed[uname]
	if !has || len(userUsed) >= len(p.proxyListActive) {
		userUsed = make(map[string]int64)
	}
	p.proxyUsed[uname] = userUsed

	for _, proxy := range p.proxyListActive {
		if _, has := userUsed[proxy.proxy]; has {
			continue
		}
		proxy.Used++
		userUsed[proxy.proxy] = time.Now().Unix()
		return proxy, nil
	}
	return nil, errorNoProxy
}

func (p *ProxyPool) runTest() {
	p.testRunChan <- true
	defer (func() {
		<-p.testRunChan
	})()
	start := time.Now()
	proxyTotal := len(p.proxyListAll)
	log.Println("start test all proxy, total=", proxyTotal)

	var wg sync.WaitGroup
	for name := range p.proxyListAll {
		wg.Add(1)
		go (func(proxyUrl string) {
			p.testProxyAddActive(proxyUrl)
			wg.Done()
		})(name)
	}
	wg.Wait()

	used := time.Now().Sub(start)
	log.Println("test all proxy finish, total:", proxyTotal, "used:", used, "activeTotal:", len(p.proxyListActive))

	p.cleanBadProxy(24 * time.Hour)

	testResultFile := filepath.Join(fsenv.ConfRootDir(), "pool_checked.conf")
	fs.FilePutContents(testResultFile, []byte(p.String()))
}

// testProxyAddActive 测试一个代理是否可用 若可用则加入代理池否则删除
func (p *ProxyPool) testProxyAddActive(proxyURL string) bool {
	proxy := p.getProxy(proxyURL)
	if proxy == nil {
		return false
	}
	isOk := p.testProxy(proxy)
	if isOk {
		p.addProxyActive(proxy.proxy)
	} else {
		p.removeProxyActive(proxy.proxy)
	}
	return true
}

func (p *ProxyPool) testProxy(proxy *Proxy) bool {
	p.checkChan <- proxy.proxy
	start := time.Now()
	defer (func() {
		<-p.checkChan
	})()

	if start.Sub(proxy.LastCheck) < p.config.getCheckInterval() {
		return proxy.IsOk()
	}

	proxy.StatusCode = proxyStatusUnavailable

	testlog := func(msg ...interface{}) {
		proxy.CheckUsed = time.Now().Sub(start)
		proxy.LastCheck = start
		log.Println("test proxy", proxy.proxy, fmt.Sprint(msg...), "used:", proxy.CheckUsed, "ms")
	}

	checkURL := p.config.getAliveCheckURL()
	if len(checkURL) > 0 {
		resp, err := doRequestGet(checkURL, proxy, p.config.getTimeout())
		if err != nil {
			testlog("failed,", err.Error())
			return false
		}
		curLen := resp.Header.Get("Content-Length")
		checkLen := p.aliveCheckResponse.Header.Get("Content-Length")
		if curLen != checkLen {
			testlog("failed, content-length wrong, [", checkLen, "!=", curLen, "]")
			return false
		}
	} else {
		host, port, err := getHostPortFromURL(proxy.proxy)
		if err != nil {
			testlog("failed, proxy url err:", err)
			return false
		}
		address := net.JoinHostPort(host, strconv.Itoa(port))
		conn, netErr := net.DialTimeout("tcp", address, p.config.getTimeout())
		if netErr != nil {
			testlog("failed", netErr)
			return false
		}
		_ = conn.Close()
	}
	proxy.StatusCode = proxyStatusActive
	proxy.LastCheckOk = time.Now()
	testlog("pass")
	return true
}

// markProxyStatus 标记一个代理的连接状态
func (p *ProxyPool) markProxyStatus(proxy *Proxy, status proxyUsedStatus) {
	proxy.Count.MarkStatus(status)
	p.Count.MarkStatus(status)
}

// GetProxyNumbers 返回各种代理的数量 web页面会使用
func (p *ProxyPool) GetProxyNumbers() GroupNumbers {
	data := newGroupNumbers()
	data.Add("total", len(p.proxyListAll))
	data.Add("active", len(p.proxyListActive))
	for _type := range proxyTransports {
		name := fmt.Sprintf("active_%s", _type)
		data.Add(name, 0)
	}
	for _, proxy := range p.proxyListActive {
		name := fmt.Sprintf("active_%s", proxy.URL.Scheme)
		data.Add(name, 1)
	}
	return data
}

func doRequestGet(urlStr string, proxy *Proxy, timeout time.Duration) (resp *http.Response, err error) {
	client := &http.Client{}
	if proxy != nil {
		client, err = newClient(proxy.URL, timeout)
		if err != nil {
			return nil, err
		}
	}
	defer client.CloseIdleConnections()
	client.Timeout = timeout
	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func (p *ProxyPool) cleanBadProxy(dur time.Duration) {
	last := time.Now().Add(-1 * dur)
	var proxyBad []*Proxy
	for _, proxy := range p.proxyListAll {
		if proxy.LastCheckOk.Before(last) {
			proxyBad = append(proxyBad, proxy)
		}
	}

	for _, proxy := range proxyBad {
		p.removeProxy(proxy.proxy)
		fp := filepath.Join(fsenv.ConfRootDir(), "pool_bad.list")
		fs.FilePutContents(fp, []byte(proxy.String()+"\n"), fs.FILE_APPEND)
	}
}

func (p *ProxyPool) cleanProxyUsed() {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now().Unix()
	var delNames []string
	for name, infos := range p.proxyUsed {
		var delKeys []string
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
		delete(p.proxyUsed, name)
		log.Println("cleanProxyUsed name=", name)
	}
}

func (p *ProxyPool) ActiveList() map[string]*Proxy {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.proxyListActive
}
