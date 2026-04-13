//  Copyright(C) 2026 github.com/hidu  All Rights Reserved.
//  Author: hidu <duv123+git@gmail.com>
//  Date: 2026-04-12

package transport

import (
	"context"
	"fmt"
	"net"
	"net/url"
)

type Transporter struct {
	Dial        func(network, addr string) (net.Conn, error)
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

func (t *Transporter) Connect(ctx context.Context, network, addr string) (net.Conn, error) {
	if t.DialContext != nil {
		return t.DialContext(ctx, network, addr)
	}
	return t.Dial(network, addr)
}

var registry = make(map[string]func(proxyURL *url.URL) *Transporter)

func Get(proxyURL *url.URL) (*Transporter, error) {
	gf, ok := registry[proxyURL.Scheme]
	if !ok {
		return nil, fmt.Errorf("connot find proxy scheme: %s", proxyURL.Scheme)
	}
	return gf(proxyURL), nil
}
