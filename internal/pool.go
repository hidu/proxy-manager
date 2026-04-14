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

	"github.com/xanygo/anygo/ds/xslice"
	"github.com/xanygo/anygo/ds/xsync"
	"github.com/xanygo/anygo/safely"
	"github.com/xanygo/anygo/xattr"
	"github.com/xanygo/anygo/xerror"
	"github.com/xanygo/anygo/xlog"
)

// 支持动态修改的代理配置列表
const dynCfgName = "dyn.yml"

func dynCfgPath() string {
	return filepath.Join(xattr.ConfDir(), dynCfgName)
}

// ProxyPool 代理池
type ProxyPool struct {
	checkerJobs chan *proxyEntry

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
		checkerJobs: make(chan *proxyEntry, 8),
		all:         newProxyList(nil),
		active:      newProxyList(nil),
		primary:     newProxyList(nil),
		dyn:         newProxyList(nil),
	}

	p.loadProxies()

	p.startCheckWorkers()

	SetInterval(p.startCheckProducer, getCheckInterval())
	go p.startCheckProducer()

	SetInterval(p.trySaveToFile, 2*time.Second)

	return p
}

// loadProxies 加载所有配置文件
func (p *ProxyPool) loadProxies() {
	// load conf/proxies.yml
	{
		pl, err := p.parserConfigFile("proxies")
		if err != nil {
			log.Printf("load conf/proxies failed: %v, ignored\n", err)
		}
		log.Printf("found %d proxies in conf/proxies\n", pl.Total())
		pl.MergeTo(p.primary)
		pl.MergeTo(p.all)
	}

	// load conf/dyn.yml
	{
		temp, err := p.parserConfigFile(dynCfgName)
		if err == nil && temp != nil {
			temp.MergeTo(p.dyn)
			temp.MergeTo(p.all)
		}
	}

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

var errorNoProxy = errors.New("no active proxy")

func (p *ProxyPool) getOneProxyActive(uname string) (*proxyEntry, error) {
	one := p.active.Next()
	if one == nil {
		return nil, errorNoProxy
	}
	return one, nil
}

var producerRunning atomic.Bool

func (p *ProxyPool) startCheckProducer() {
	if !producerRunning.CompareAndSwap(false, true) {
		xlog.Warn(context.Background(), "CheckProducer already running, skipped")
		return
	}
	defer producerRunning.Store(false)

	start := time.Now()
	items := p.all.All()

	xlog.Info(context.Background(), "CheckProducer starting...", xlog.Int("TotalJobs", len(items)))

	var isCanceled bool

	for _, one := range items {
		if time.Now().Before(silentDeadline.Load()) {
			isCanceled = true
			break
		}
		p.checkerJobs <- one
	}
	cost := time.Since(start)
	xlog.Info(context.Background(), "CheckProducer done",
		xlog.Int("TotalJobs", len(items)),
		xlog.DurationMS("Cost", cost),
		xlog.Bool("IsCanceled", isCanceled),
		xlog.Time("Start", start),
	)
}

func (p *ProxyPool) startCheckWorkers() {
	for i := 0; i < 8; i++ {
		go safely.Run(p.checkWorker)
	}
}

func (p *ProxyPool) checkWorker() {
	for one := range p.checkerJobs {
		p.testProxyAddActive(one)
	}
}

// 尝试保存文件
func (p *ProxyPool) trySaveToFile() {
	// conf/dyn.yml
	{
		changed := p.dyn.changed.Load()
		if !changed.IsZero() && time.Since(changed) > time.Second {
			p.dyn.ResetChanged()
			p.dyn.SaveFile(dynCfgPath())
		}
	}

	// {temp}/active_proxies.yml
	{
		changed := p.active.changed.Load()
		if !changed.IsZero() && time.Since(changed) > time.Second {
			p.active.ResetChanged()
			filename := filepath.Join(xattr.TempDir(), "active_proxies.yml")
			p.active.SaveFile(filename)
		}
	}
}

// testProxyAddActive 测试一个代理是否可用 若可用则加入代理池否则删除
func (p *ProxyPool) testProxyAddActive(one *proxyEntry) {
	if p.checkProxyEntry(one) {
		p.active.Add(one)
	} else {
		p.active.Remove(one)
	}
}

var lastChecked xsync.Value[string]

func (p *ProxyPool) checkProxyEntry(proxy *proxyEntry) bool {
	if p.all.Get(proxy.Base.Proxy) == nil {
		return false
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), getProxyTimeout())
	defer cancel()
	ctx = xlog.NewContext(ctx)

	xlog.AddAttr(ctx, xlog.String("Proxy", proxy.Base.Proxy))

	checkURL := getProbeURL()
	resp, err := httpGetByProxyEntry(ctx, checkURL, proxy)
	{
		cost := time.Since(start)
		proxy.State.LastCheckUsed.Store(cost)
		proxy.State.LastCheck.Store(start)
	}
	if err != nil {
		proxy.State.LastCheckStatus.Store(int64(255))
		proxy.State.LastCheckMsg.Store(err.Error())
		xlog.Warn(ctx, "checkProxy failed", xlog.ErrorAttr("error", err))

		lastChecked.Store(fmt.Sprintf("%s: %s >err: %s", time.Now().String(), proxy.Base.URL.Hostname(), err.Error()))

		if pool.dyn.Remove(proxy) {
			pool.all.Remove(proxy)
		}
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	proxy.State.LastCheckStatus.Store(int64(resp.StatusCode))
	proxy.State.LastCheckMsg.Store("")

	lastChecked.Store(fmt.Sprintf("%s: %s >status: %s", time.Now().String(), proxy.Base.URL.Hostname(), resp.Status))

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		proxy.State.LastCheckOk.Store(start)
		xlog.Info(ctx, "checkProxy success")
		return true
	}

	xlog.Warn(ctx, "checkProxy failed", xlog.Int("StatusCode", resp.StatusCode))
	return false
}

var dynCleanRunning atomic.Bool

var dynCleanLimiter = make(chan struct{}, 8)

// DynClean 清理 dyn 里无无效的配置
func (p *ProxyPool) DynClean(limit int, timeout int) any {
	dynCleanRunning.Store(true)
	defer dynCleanRunning.Store(false)

	result := make([]any, 0)
	var mux sync.Mutex

	ctx := context.Background()
	var deletedTotal atomic.Int32
	var checked atomic.Int32

	// 分批检查
	check := func(list []*proxyEntry) {
		var deleted atomic.Int32
		var wg xsync.WaitGroup
		for _, proxy := range list {
			wg.Go(func() {
				dynCleanLimiter <- struct{}{}
				defer func() {
					<-dynCleanLimiter
				}()
				start := time.Now()
				err := proxy.TestByDial(ctx, timeout)
				cost := time.Since(start)
				if err != nil {
					p.dyn.Remove(proxy)
					p.all.Remove(proxy)

					deleted.Add(1)
					deletedTotal.Add(1)
				}
				xlog.Info(ctx, "TestByDial",
					xlog.String("Proxy", proxy.Base.URL.Host),
					xlog.ErrorAttr("Error", err),
					xlog.DurationMS("Cost", cost),
				)

				mux.Lock()
				info := map[string]any{
					"Proxy":  proxy.Base.Proxy,
					"Cost":   cost.String(),
					"Delete": err != nil,
					"Err":    xerror.String(err),
				}
				result = append(result, info)
				mux.Unlock()

				checked.Add(1)
			})
		}
		wg.Wait()
	}

	start := time.Now()

	itemsChunk := xslice.Chunk(p.dyn.All(), 16)
	for _, pps := range itemsChunk {
		if time.Now().Before(silentDeadline.Load()) {
			break
		}
		check(pps)
		if limit > 0 && checked.Load() >= int32(limit) {
			break
		}
	}

	total := map[string]any{
		"Deleted": deletedTotal.Load(),
		"Cost":    time.Since(start).String(),
	}
	result = append(result, total)
	return result
}
