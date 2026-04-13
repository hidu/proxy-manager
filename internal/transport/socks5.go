//  Copyright(C) 2026 github.com/hidu  All Rights Reserved.
//  Author: hidu <duv123+git@gmail.com>
//  Date: 2026-04-12

package transport

import (
	"context"
	"net"
	"net/url"

	"golang.org/x/net/proxy"
)

type ctxDialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

func genSocks5(proxyURL *url.URL) *Transporter {
	return &Transporter{
		Dial: func(network, addr string) (net.Conn, error) {
			ph, err := proxy.FromURL(proxyURL, proxy.Direct)
			if err != nil {
				return nil, err
			}
			return ph.Dial(network, addr)
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			ph, err := proxy.FromURL(proxyURL, proxy.Direct)
			if err != nil {
				return nil, err
			}
			if dc, ok := ph.(ctxDialer); ok {
				return dc.DialContext(ctx, network, addr)
			}
			return ph.Dial(network, addr)
		},
	}
}

func init() {
	registry["socks5"] = genSocks5
}
