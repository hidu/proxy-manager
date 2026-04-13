//  Copyright(C) 2026 github.com/hidu  All Rights Reserved.
//  Author: hidu <duv123+git@gmail.com>
//  Date: 2026-04-12

package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/xanygo/anygo/xlog"
)

var zd net.Dialer

func httpProxyDialer(proxyURL *url.URL) func(ctx context.Context, network, addr string) (net.Conn, error) {
	host := proxyURL.Hostname()
	port := proxyURL.Port()
	if port == "" {
		switch proxyURL.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	serverAddr := net.JoinHostPort(host, port)
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid target address %q: %v", addr, err)
		}
		buf := &bytes.Buffer{}
		fmt.Fprintf(buf, "%s %s HTTP/1.1\r\n", http.MethodConnect, addr)
		fmt.Fprintf(buf, "Host: %s\r\n", proxyURL.Host)
		fmt.Fprintf(buf, "User-Agent: %s\r\n", "Hello")
		buf.WriteString("Proxy-Connection: keep-alive\r\n")
		if user := proxyURL.User; user != nil {
			code := base64.StdEncoding.EncodeToString([]byte(user.String()))
			buf.WriteString("Proxy-Authorization: Basic " + code + "\r\n")
		}
		buf.WriteString("\r\n")

		conn, err := zd.DialContext(ctx, network, serverAddr)
		if err != nil {
			return nil, err
		}
		_, err = conn.Write(buf.Bytes())
		if err != nil {
			return nil, fmt.Errorf("send Connect request failed: %w", err)
		}
		bio := bufio.NewReader(conn)
		resp, err := http.ReadResponse(bio, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		xlog.Info(ctx, "connected to proxy server", xlog.String("Status", resp.Status))
		if resp.StatusCode != http.StatusOK {
			conn.Close()
			return nil, fmt.Errorf("bad proxy response status: %d  %s", resp.StatusCode, resp.Status)
		}
		return conn, nil
	}
}

func genHTTP(proxyURL *url.URL) *Transporter {
	return &Transporter{
		DialContext: httpProxyDialer(proxyURL),
	}
}

func init() {
	registry["http"] = genHTTP
	registry["https"] = genHTTP
}
