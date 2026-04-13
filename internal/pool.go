package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xanygo/anygo/ds/xsync"
	"github.com/xanygo/anygo/xattr"
	"github.com/xanygo/anygo/xerror"
	"github.com/xanygo/anygo/xio/xfs"
	"github.com/xanygo/anygo/xlog"
)

// 支持动态修改的代理配置列表
const dynCfgName = "dyn.yml"

func dynCfgPath() string {
	return filepath.Join(xattr.ConfDir(), dynCfgName)
}

// ProxyPool 代理池
type ProxyPool struct {
	limiter        chan struct{} // 限制并发度
	checkerRunning atomic.Bool   // 运行中标记

	active *ProxyList // 活跃可用的
	all    *ProxyList // 所有的

	primary *ProxyList // 主配置，由 conf/proxies.yml 加载而来

	// 动态配置，由 conf/dyn.yml 加载而来
	dyn *ProxyList
}

var pool *ProxyPool

// loadPool 从配置文件中加载代理池
func loadPool() *ProxyPool {
	p := &ProxyPool{
		limiter: make(chan struct{}, 64),
		all:     newProxyList(nil),
		active:  newProxyList(nil),
		primary: newProxyList(nil),
		dyn:     newProxyList(nil),
	}

	p.loadProxies()

	go p.runTest()

	SetInterval(func() {
		p.runTest()
	}, getCheckInterval())

	return p
}

// loadProxies 加载所有配置文件
func (p *ProxyPool) loadProxies() {
	pl, err := p.parserConfigFile("proxies") // 加载 conf/proxies.yml 陪
	if err != nil {
		log.Printf("load conf/proxies failed: %v, ignored\n", err)
	}
	log.Printf("found %d proxies in conf/proxies\n", pl.Total())
	pl.MergeTo(p.primary)
	pl.MergeTo(p.all)

	wf := &xfs.WatchFile{
		FileName: dynCfgPath(),
		Parser: func(path string) error {
			temp, err := p.parserConfigFile(dynCfgName)
			if err == nil && temp != nil {
				temp.MergeTo(p.dyn)
				temp.MergeTo(p.all)
			}
			return err
		},
	}
	wf.Start()

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
	p.SaveActiveToFile()
}

func (p *ProxyPool) SaveActiveToFile() {
	filename := filepath.Join(xattr.TempDir(), "active_proxies.yml")
	p.active.SaveFile(filename)
}

func (p *ProxyPool) SaveTempToFile() {
	p.dyn.SaveFile(dynCfgPath())
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

// DynClean 清理 dyn 里无无效的配置
func (p *ProxyPool) DynClean() any {
	result := make([]any, 0)
	var mux sync.Mutex

	ctx := context.Background()
	var wg xsync.WaitGroup
	limiter := make(chan struct{}, 8)
	var deleted atomic.Int32
	p.dyn.Range(func(proxyURL string, proxy *proxyEntry) bool {
		wg.Go(func() {
			limiter <- struct{}{}
			start := time.Now()
			err := proxy.TestByDial(ctx)
			cost := time.Since(start)
			if err != nil {
				p.dyn.Remove(proxy)
				p.all.Remove(proxy)

				deleted.Add(1)
			}
			xlog.Info(ctx, "TestByDial",
				xlog.String("Proxy", proxy.Base.URL.Host),
				xlog.ErrorAttr("Error", err),
				xlog.DurationMS("Cost", cost),
			)

			mux.Lock()
			info := map[string]any{
				"Proxy":  proxyURL,
				"Cost":   cost.String(),
				"Delete": err != nil,
				"Err":    xerror.String(err),
			}
			result = append(result, info)
			mux.Unlock()

			<-limiter
		})
		return true
	})
	wg.Wait()

	if deleted.Load() > 0 {
		p.SaveTempToFile()
	}
	total := map[string]any{
		"Deleted": deleted.Load(),
	}
	result = append(result, total)
	return result
}
