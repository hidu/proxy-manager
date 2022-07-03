package internal

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type proxyStatus int

const (
	proxyStatusUnknown proxyStatus = iota
	proxyStatusActive
	proxyStatusUnavailable
)

func (status proxyStatus) String() string {
	switch status {
	case proxyStatusUnknown:
		return "unknown"
	case proxyStatusActive:
		return "active"
	case proxyStatusUnavailable:
		return "unavailable"
	}
	return fmt.Sprintf("unknow status:%d", status)
}

type proxyUsedStatus int

const (
	proxyUsedSuc proxyUsedStatus = iota
	proxyUsedFailed
)

func (status proxyUsedStatus) String() string {
	switch status {
	case proxyUsedSuc:
		return "success"
	case proxyUsedFailed:
		return "failed"
	}
	return fmt.Sprintf("unknow status:%d", status)
}

// Proxy 一个代理
type Proxy struct {
	proxy       string
	URL         *url.URL
	Weight      int
	StatusCode  proxyStatus
	CheckUsed   time.Duration //
	LastCheck   time.Time
	LastCheckOk time.Time
	Used        int64
	Count       *proxyCount
}

func newProxy(proxyURL string) *Proxy {
	proxy := &Proxy{proxy: proxyURL}
	var err error
	proxy.URL, err = url.Parse(proxyURL)
	if err != nil {
		log.Println("proxy info wrong", err)
		return nil
	}
	proxy.Weight = 0
	proxy.Count = newProxyCount()
	return proxy
}

func (p *Proxy) String() string {
	return fmt.Sprintf("proxy=%-40s\tweight=%d\tlast_check=%d\tcheck_used=%s\tstatus=%d\tlast_check_ok=%d",
		p.proxy,
		p.Weight,
		p.LastCheck.Unix(),
		p.CheckUsed,
		p.StatusCode,
		p.LastCheckOk.Unix(),
	)
}

// IsOk 是否可用状态
func (p *Proxy) IsOk() bool {
	return p.StatusCode == proxyStatusActive
}

func (p *Proxy) IncrUsed() {
	atomic.AddInt64(&p.Used, 1)
}

type ProxyList struct {
	list sync.Map
}

func (pl *ProxyList) Range(fn func(proxyURL string, proxy *Proxy) bool) {
	pl.list.Range(func(key, value any) bool {
		return fn(key.(string), value.(*Proxy))
	})
}

func (pl *ProxyList) Add(p *Proxy) bool {
	_, loaded := pl.list.LoadOrStore(p.proxy, p)
	return !loaded
}

func (pl *ProxyList) Remove(key string) bool {
	_, loaded := pl.list.LoadAndDelete(key)
	return loaded
}

func (pl *ProxyList) Get(key string) *Proxy {
	val, has := pl.list.Load(key)
	if !has {
		return nil
	}
	return val.(*Proxy)
}

func (pl *ProxyList) Total() int {
	var total int
	pl.list.Range(func(_, _ any) bool {
		total++
		return true
	})
	return total
}

func (pl *ProxyList) MergeTo(to ProxyList) {
	pl.list.Range(func(_, value any) bool {
		to.Add(value.(*Proxy))
		return true
	})
}

func (pl *ProxyList) String() string {
	var all []string
	pl.list.Range(func(_, value any) bool {
		p := value.(*Proxy)
		all = append(all, p.String())
		return true
	})
	return strings.Join(all, "\n")
}
