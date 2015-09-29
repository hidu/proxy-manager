package manager

import (
	"fmt"
	"log"
	"net/url"
)

type proxyStatus int

const (
	proxyStatusUnknow proxyStatus = iota
	proxyStatusActive
	proxyStatusUnavaliable
)

func (status proxyStatus) String() string {
	switch status {
	case proxyStatusUnknow:
		return "unknow"
	case proxyStatusActive:
		return "active"
	case proxyStatusUnavaliable:
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
	CheckUsed   int64 //ms
	LastCheck   int64
	LastCheckOk int64
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

func (proxy *Proxy) String() string {
	return fmt.Sprintf("proxy=%-40s\tweight=%d\tlast_check=%d\tcheck_used=%d\tstatus=%d\tlast_check_ok=%d",
		proxy.proxy,
		proxy.Weight,
		proxy.LastCheck,
		proxy.CheckUsed,
		proxy.StatusCode,
		proxy.LastCheckOk,
	)
}

// IsOk 是否可用状态
func (proxy *Proxy) IsOk() bool {
	return proxy.StatusCode == proxyStatusActive
}
