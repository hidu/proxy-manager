//  Copyright(C) 2026 github.com/hidu  All Rights Reserved.
//  Author: hidu <duv123+git@gmail.com>
//  Date: 2026-04-12

package transport

import (
	"net"
	"net/url"

	"h12.io/socks"
)

func genSocks4(proxyURL *url.URL) *Transporter {
	return &Transporter{
		Dial: func(network, addr string) (net.Conn, error) {
			dialFn := socks.DialSocksProxy(socks.SOCKS4A, proxyURL.Host)
			return dialFn(network, addr)
		},
	}
}

func init() {
	registry["socks4"] = genSocks4
	registry["socks4a"] = genSocks4
}
