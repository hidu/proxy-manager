package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xanygo/anygo/xattr"
	"github.com/xanygo/anygo/xio/xfs"
	"github.com/xanygo/anygo/xlog"
)

// ProxyPool 代理池
type ProxyPool struct {
	limiter        chan struct{} // 限制并发度
	checkerRunning atomic.Bool   // 运行中标记

	active *ProxyList // 活跃可用的
	all    *ProxyList // 所有的
}

var pool *ProxyPool

// loadPool 从配置文件中加载代理池
func loadPool() *ProxyPool {
	p := &ProxyPool{
		limiter: make(chan struct{}, 100),
		all:     newProxyList(nil),
		active:  newProxyList(nil),
	}

	p.loadToAll("proxies.yml")

	go p.runTest()

	SetInterval(func() {
		p.runTest()
	}, getCheckInterval())

	return p
}

func (p *ProxyPool) loadToAll(name string) {
	pl, err := p.parserConfigFile(name)
	if err != nil {
		log.Printf("parser %s failed: %v, ignored\n", name, err)
	}
	log.Printf("found %d proxies in %s\n", pl.Total(), name)
	pl.MergeTo(p.all)
	log.Println("p.all=", p.all.Total())
}

func (p *ProxyPool) String() string {
	return p.all.String()
}

func (p *ProxyPool) parserConfigFile(confName string) (*ProxyList, error) {
	items, err := loadProxies(confName)
	if err != nil {
		return nil, err
	}
	return newProxyList(items), nil
}

func (p *ProxyPool) addProxy(proxy *proxyEntry) bool {
	return p.all.Add(proxy)
}

func (p *ProxyPool) getProxy(proxyURL string) *proxyEntry {
	return p.all.Get(proxyURL)
}

var errorNoProxy = errors.New("no active proxy")

func (p *ProxyPool) getOneProxyActive(uname string) (*proxyEntry, error) {
	if p.active.Total() == 0 {
		return nil, errorNoProxy
	}

	one := p.active.Next()

	if one == nil {
		return nil, errorNoProxy
	}
	return one, nil
}

func (p *ProxyPool) runTest() {
	if !p.checkerRunning.CompareAndSwap(false, true) {
		xlog.Info(context.Background(), "checker already is running")
		return
	}

	proxyTotal := p.all.Total()

	xlog.Info(context.Background(), "start test all proxy", xlog.Int("total", proxyTotal))

	if proxyTotal == 0 {
		return
	}
	start := time.Now()

	var wg sync.WaitGroup
	p.all.Range(func(proxyURL string, proxy *proxyEntry) bool {
		wg.Go(func() {
			p.testProxyAddActive(proxyURL)
		})
		return true
	})
	wg.Wait()

	used := time.Since(start)

	xlog.Info(context.Background(), "test all proxy finished",
		xlog.Int("Total", proxyTotal),
		xlog.Int("Active", len(p.ActiveList())),
		xlog.DurationMS("Cost", used),
	)

	testResultFile := filepath.Join(xattr.TempDir(), "active_proxies.yml")
	xfs.KeepDirExists(filepath.Dir(testResultFile))
	all := p.active.String()
	_ = os.WriteFile(testResultFile, []byte(all), 0666)
}

// testProxyAddActive 测试一个代理是否可用 若可用则加入代理池否则删除
func (p *ProxyPool) testProxyAddActive(proxyURL string) bool {
	one := p.getProxy(proxyURL)
	if one == nil {
		return false
	}
	isOk := p.testProxy(one)
	if isOk {
		p.active.Add(one)
	} else {
		p.active.Remove(one)
	}
	return true
}

func (p *ProxyPool) testProxy(proxy *proxyEntry) bool {
	start := time.Now()
	if start.Sub(proxy.State.LastCheck.Load()) < getCheckInterval() {
		return proxy.IsOk()
	}

	p.limiter <- struct{}{}
	defer func() {
		<-p.limiter
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = xlog.NewContext(ctx)

	xlog.AddAttr(ctx, xlog.String("Proxy", proxy.Base.Proxy))

	checkURL := getAliveCheckURL()
	resp, err := httpGetByProxyEntry(ctx, checkURL, proxy)
	{
		cost := time.Since(start)
		proxy.State.LastCheckUsed.Store(cost)
		proxy.State.LastCheck.Store(start)
	}
	if err != nil {
		proxy.State.LastCheckStatus.Store(int64(255))
		proxy.State.LastCheckMsg.Store(err.Error())
		xlog.Warn(ctx, "testProxy failed", xlog.ErrorAttr("error", err))
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	proxy.State.LastCheckStatus.Store(int64(resp.StatusCode))
	proxy.State.LastCheckMsg.Store("")

	if resp.StatusCode == http.StatusOK {
		proxy.State.LastCheckOk.Store(start)
		xlog.Info(ctx, "testProxy success")
		return true
	}

	xlog.Warn(ctx, "testProxy failed", xlog.Int("StatusCode", resp.StatusCode))
	return false
}

// GetProxyNumbers 返回各种代理的数量 web页面会使用
func (p *ProxyPool) GetProxyNumbers() map[string]int {
	data := make(map[string]int, 10)
	data["total"] = p.all.Total()
	data["active"] = p.active.Total()

	p.active.Range(func(proxyURL string, proxy *proxyEntry) bool {
		name := fmt.Sprintf("active_%s", proxy.Base.URL.Scheme)
		data[name]++
		return true
	})
	return data
}

func (p *ProxyPool) ActiveList() map[string]*proxyEntry {
	proxies := make(map[string]*proxyEntry)
	p.active.Range(func(proxyURL string, proxy *proxyEntry) bool {
		proxies[proxyURL] = proxy
		return true
	})
	return proxies
}

func (p *ProxyPool) All() []*proxyEntry {
	return p.all.All()
}
