package manager

import (
	"code.google.com/p/go.net/proxy"
	"fmt"
	//"github.com/hailiang/socks"
	ss "github.com/shadowsocks/shadowsocks-go/shadowsocks"
	"h12.me/socks"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var proxyTransports = make(map[string]func(proxyUrl *url.URL) (*http.Transport, error))

func init() {

	proxyTransports["http"] = func(proxyURL *url.URL) (*http.Transport, error) {
		return &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return proxyURL, nil
			},
		}, nil
	}

	proxyTransports["socks5"] = func(proxyURL *url.URL) (*http.Transport, error) {
		ph, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return nil, err
		}
		return &http.Transport{
			Dial: ph.Dial,
		}, nil
	}

	proxyTransports["socks4"] = func(proxyURL *url.URL) (*http.Transport, error) {
		dialSocksProxy := socks.DialSocksProxy(socks.SOCKS4A, proxyURL.Host)
		return &http.Transport{
			Dial: dialSocksProxy,
		}, nil
	}

	proxyTransports["socks4a"] = func(proxyURL *url.URL) (*http.Transport, error) {
		dialSocksProxy := socks.DialSocksProxy(socks.SOCKS4A, proxyURL.Host)
		return &http.Transport{
			Dial: dialSocksProxy,
		}, nil
	}

	//shadowsocks
	proxyTransports["ss"] = func(proxyURL *url.URL) (*http.Transport, error) {
		if proxyURL.User == nil {
			return nil, fmt.Errorf("wrong shadowsocks uri,need method and passwd")
		}
		psw, _ := proxyURL.User.Password()
		cipher, err := ss.NewCipher(proxyURL.User.Username(), psw)
		if err != nil {
			return nil, err
		}
		serverAddr := proxyURL.Host
		return &http.Transport{
			Dial: func(_, addr string) (net.Conn, error) {
				return ss.Dial(addr, serverAddr, cipher.Copy())
			},
		}, nil
	}

}

func newClient(proxyURL *url.URL, timeout int) (*http.Client, error) {
	client := &http.Client{}
	client.Timeout = time.Duration(timeout) * time.Second

	if transFn, has := proxyTransports[strings.ToLower(proxyURL.Scheme)]; has {
		tr, err := transFn(proxyURL)
		if err != nil {
			return nil, err
		}
		client.Transport = tr
		return client, nil
	}
	return nil, fmt.Errorf("unknow proxy scheme:%s", proxyURL.Scheme)
}
