package internal

import (
	"context"
	"net"
	"net/http"
	"net/url"

	"github.com/hidu/proxy-manager/internal/transport"
)

func httpClientProxied(proxy *url.URL) (*http.Client, error) {
	c := &http.Client{}
	tr, err := transport.Get(proxy)
	if err != nil {
		return nil, err
	}
	htr := &http.Transport{
		DialContext: tr.DialContext,
		Dial:        tr.Dial,
	}
	if tr.DialContext != nil {
		htr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			ctx, cancel := context.WithTimeout(ctx, getProxyTimeout())
			defer cancel()
			return tr.DialContext(ctx, network, addr)
		}
	}
	c.Transport = htr
	return c, nil
}

func httpGetByProxyEntry(ctx context.Context, urlStr string, proxy *proxyEntry) (resp *http.Response, err error) {
	return httpGetByProxyURL(ctx, urlStr, proxy.Base.URL)
}

func httpGetByProxyURL(ctx context.Context, urlStr string, proxy *url.URL) (resp *http.Response, err error) {
	c, err := httpClientProxied(proxy)
	if err != nil {
		return nil, err
	}
	defer c.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}
