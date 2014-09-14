package manager

import (
	"fmt"
	"log"
	"net/url"
)

type PROXY_STATUS int

const (
	PROXY_STATUS_UNKNOW PROXY_STATUS = iota
	PROXY_STATUS_ACTIVE
	PROXY_STATUS_UNAVAILABLE
)

func (status PROXY_STATUS) String() string {
	switch status {
	case PROXY_STATUS_UNKNOW:
		return "unknow"
	case PROXY_STATUS_ACTIVE:
		return "active"
	case PROXY_STATUS_UNAVAILABLE:
		return "unavailable"
	}
	return fmt.Sprintf("unknow status:%d", status)
}

type PROXY_USED_STATUS int

const (
	PROXY_USED_SUC PROXY_USED_STATUS = iota
	PROXY_USED_FAILED
)

func (status PROXY_USED_STATUS) String() string {
	switch status {
	case PROXY_USED_SUC:
		return "success"
	case PROXY_USED_FAILED:
		return "failed"
	}
	return fmt.Sprintf("unknow status:%d", status)
}

type Proxy struct {
	proxy      string
	URL        *url.URL
	Weight     int
	StatusCode PROXY_STATUS
	CheckUsed  int64 //ms
	LastCheck  int64
	Used       int64
	Count      *ProxyCount
}

func NewProxy(proxyUrl string) *Proxy {
	proxy := &Proxy{proxy: proxyUrl}
	var err error
	proxy.URL, err = url.Parse(proxyUrl)
	if err != nil {
		log.Println("proxy info wrong", err)
		return nil
	}
	proxy.Weight = 0
	proxy.Count = NewProxyCount()
	return proxy
}

func (proxy *Proxy) String() string {
	return fmt.Sprintf("proxy=%-40s\tweight=%d\tlast_check=%d\tcheck_used=%d\tstatus=%d",
		proxy.proxy,
		proxy.Weight,
		proxy.LastCheck,
		proxy.CheckUsed,
		proxy.StatusCode,
	)
}

func (proxy *Proxy) IsOk() bool {
	return proxy.StatusCode == PROXY_STATUS_ACTIVE
}
