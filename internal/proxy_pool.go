package internal

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/fsgo/fsenv"
	"github.com/hidu/goutils/fs"
	"github.com/hidu/goutils/str_util"
)

// ProxyPool 代理池
type ProxyPool struct {
	activeList ProxyList // 活跃可用的
	allList    ProxyList // 所有的

	config *Config

	aliveCheckResponse *http.Response

	checkChan   chan string
	testRunChan chan bool

	Count *proxyCount
}

// loadPool 从配置文件中加载代理池
func loadPool(cfg *Config) *ProxyPool {
	log.Println("loading proxy pool...")
	pool := &ProxyPool{
		config:      cfg,
		checkChan:   make(chan string, 100),
		testRunChan: make(chan bool, 1),
		Count:       newProxyCount(),
	}

	checkURL := pool.config.getAliveCheckURL()

	if len(checkURL) > 0 {
		var err error
		pool.aliveCheckResponse, err = doRequestGet(checkURL, nil, pool.config.getTimeout())
		if err != nil {
			// todo allow fail
			log.Fatalln("get alive response failed, url:", checkURL, "err:", err)
			return nil
		}
		log.Println("get alive info success! url:", checkURL, "resp_header:", pool.aliveCheckResponse.Header)
	}

	pool.loadPoolConfToAll(confPool)
	pool.loadPoolConfToAll(confPoolChecked)

	if pool.allList.Total() == 0 {
		log.Println("proxy pool list is empty")
	}

	go pool.runTest()

	SetInterval(func() {
		pool.runTest()
	}, pool.config.getCheckInterval())

	return pool
}

func (p *ProxyPool) loadPoolConfToAll(name string) {
	pl, err := p.parserConf(name)
	if err != nil {
		log.Printf("parser %s failed: %v, ignored\n", name, err)
	}
	log.Printf("found %d proxies in %s\n", pl.Total(), name)
	pl.MergeTo(&p.allList)
}

func (p *ProxyPool) String() string {
	return p.allList.String()
}

func (p *ProxyPool) parserConf(confName string) (ProxyList, error) {
	confPath := filepath.Join(fsenv.ConfRootDir(), confName)
	txtFile, err := str_util.NewTxtFile(confPath)
	if err != nil {
		return ProxyList{}, err
	}
	return p.loadProxiesFromTxtFile(txtFile)
}

func (p *ProxyPool) loadProxiesFromTxtFile(txtFile *str_util.TxtFile) (ProxyList, error) {
	defaultValues := make(map[string]string)
	defaultValues["proxy"] = "required"
	defaultValues["weight"] = "1"
	defaultValues["status"] = "1"
	defaultValues["last_check"] = "0"
	defaultValues["check_used"] = "0"
	defaultValues["last_check_ok"] = "0"

	datas, err := txtFile.KvMapSlice("=", true, defaultValues)
	if err != nil {
		return ProxyList{}, err
	}
	pl := ProxyList{}
	for _, kv := range datas {
		proxy := p.parseProxy(kv)
		if proxy != nil {
			pl.Add(proxy)
		}
	}
	return pl, nil
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
			log.Println("parse [", fieldName, "] failed, not int. err:", err)
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

func (p *ProxyPool) addProxy(proxy *Proxy) bool {
	return p.allList.Add(proxy)
}

func (p *ProxyPool) getProxy(proxyURL string) *Proxy {
	return p.allList.Get(proxyURL)
}

func (p *ProxyPool) addProxyActive(proxyURL string) bool {
	proxy := p.allList.Get(proxyURL)
	if proxy == nil {
		return false
	}
	return p.activeList.Add(proxy)
}

func (p *ProxyPool) removeProxyActive(proxyURL string) {
	p.activeList.Remove(proxyURL)
}

func (p *ProxyPool) removeProxy(proxyURL string) {
	p.allList.Remove(proxyURL)
	p.activeList.Remove(proxyURL)
}

var errorNoProxy = fmt.Errorf("no active proxy")

//
func (p *ProxyPool) getOneProxy(uname string) (*Proxy, error) {
	if p.activeList.Total() == 0 {
		return nil, errorNoProxy
	}

	proxy := p.activeList.Next()

	if proxy == nil {
		return nil, errorNoProxy
	}
	proxy.IncrUsed()
	return proxy, nil
}

func (p *ProxyPool) runTest() {
	p.testRunChan <- true
	defer (func() {
		<-p.testRunChan
	})()

	proxyTotal := p.allList.Total()
	log.Println("start test all proxy, total=", proxyTotal)
	if proxyTotal == 0 {
		return
	}
	start := time.Now()

	var wg sync.WaitGroup
	p.allList.Range(func(proxyURL string, proxy *Proxy) bool {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.testProxyAddActive(proxyURL)
		}()
		return true
	})
	wg.Wait()

	used := time.Now().Sub(start)
	log.Println("test all proxy finish, total:", proxyTotal, "used:", used, "activeTotal:", len(p.ActiveList()))

	p.cleanBadProxy(24 * time.Hour)

	testResultFile := filepath.Join(fsenv.ConfRootDir(), "pool_checked.conf")

	all := p.allList.String()
	fs.FilePutContents(testResultFile, []byte(all))
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
	data.Add("total", p.allList.Total())
	data.Add("active", p.activeList.Total())
	for _type := range proxyTransports {
		name := fmt.Sprintf("active_%s", _type)
		data.Add(name, 0)
	}
	p.activeList.Range(func(proxyURL string, proxy *Proxy) bool {
		name := fmt.Sprintf("active_%s", proxy.URL.Scheme)
		data.Add(name, 1)
		return true
	})
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

	p.allList.Range(func(proxyURL string, proxy *Proxy) bool {
		if proxy.LastCheckOk.Before(last) {
			proxyBad = append(proxyBad, proxy)
		}
		return true
	})

	for _, proxy := range proxyBad {
		p.removeProxy(proxy.proxy)
		fp := filepath.Join(fsenv.ConfRootDir(), "pool_bad.list")
		fs.FilePutContents(fp, []byte(proxy.String()+"\n"), fs.FILE_APPEND)
	}
}

func (p *ProxyPool) ActiveList() map[string]*Proxy {
	proxies := make(map[string]*Proxy)
	p.activeList.Range(func(proxyURL string, proxy *Proxy) bool {
		proxies[proxyURL] = proxy
		return true
	})
	return proxies
}
