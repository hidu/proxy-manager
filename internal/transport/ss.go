//  Copyright(C) 2026 github.com/hidu  All Rights Reserved.
//  Author: hidu <duv123+git@gmail.com>
//  Date: 2026-04-12

package transport

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/url"
	"strconv"

	"github.com/shadowsocks/go-shadowsocks2/core"
)

func genSS(proxyURL *url.URL) *Transporter {
	return &Transporter{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if proxyURL.User == nil {
				return nil, errors.New("wrong ss uri, need method and passwd")
			}
			password, _ := proxyURL.User.Password()
			method := proxyURL.User.Username()
			cipher, err := core.PickCipher(method, nil, password)
			if err != nil {
				return nil, err
			}

			// 1. 先连接 ss server
			rawConn, err := zd.DialContext(ctx, "tcp", proxyURL.Host)
			if err != nil {
				return nil, err
			}

			// 2. 包装为加密连接
			ssConn := cipher.StreamConn(rawConn)

			// 3. 写入目标地址（SOCKS5-like address）
			// SS 需要发送目标地址
			_, err = ssConn.Write(encodeSSAddr(addr))
			if err != nil {
				rawConn.Close()
				return nil, err
			}

			return ssConn, nil
		},
	}
}

func encodeSSAddr(addr string) []byte {
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	var b bytes.Buffer

	ip := net.ParseIP(host)
	if ip4 := ip.To4(); ip4 != nil {
		b.WriteByte(1) // IPv4
		b.Write(ip4)
	} else if ip6 := ip.To16(); ip6 != nil {
		b.WriteByte(4) // IPv6
		b.Write(ip6)
	} else {
		b.WriteByte(3) // domain
		b.WriteByte(byte(len(host)))
		b.WriteString(host)
	}

	_ = binary.Write(&b, binary.BigEndian, uint16(port))
	return b.Bytes()
}

func init() {
	registry["ss"] = genSS
}
