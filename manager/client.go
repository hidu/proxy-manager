package manager

import (
	"code.google.com/p/go.net/proxy"
	"fmt"
	"github.com/hailiang/socks"
	"net/http"
	"net/url"
	"time"
)

func NewClient(proxyURL *url.URL, timeout int) (*http.Client, error) {
	client := &http.Client{}
	client.Timeout = time.Duration(timeout) * time.Second

	if proxyURL.Scheme == "http" {
		client.Transport = &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return proxyURL, nil
			},
		}
	} else if proxyURL.Scheme == "socks5" {
		ph, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return nil, err
		}
		client.Transport = &http.Transport{
			Dial: ph.Dial,
		}
	} else if proxyURL.Scheme == "socks4" {
		dialSocksProxy := socks.DialSocksProxy(socks.SOCKS4, proxyURL.Host)
		client.Transport = &http.Transport{
			Dial: dialSocksProxy,
		}
	} else if proxyURL.Scheme == "socks4a" {
		dialSocksProxy := socks.DialSocksProxy(socks.SOCKS4A, proxyURL.Host)
		client.Transport = &http.Transport{
			Dial: dialSocksProxy,
		}
	} else {
		return nil, fmt.Errorf("unknow proxy scheme:%s", proxyURL.Scheme)
	}
	return client, nil
}
