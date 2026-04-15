//  Copyright(C) 2026 github.com/hidu  All Rights Reserved.
//  Author: hidu <duv123+git@gmail.com>
//  Date: 2026-04-15

package transport

import (
	"context"
	"net"
	"net/url"
)

func init() {
	registry["direct"] = func(_ *url.URL) *Transporter {
		return &Transporter{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return zd.DialContext(ctx, network, addr)
			},
		}
	}
}
