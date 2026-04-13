package internal

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xanygo/anygo/xlog"

	"github.com/hidu/proxy-manager/internal/transport"
)

var defaultRelay = &reply{}

// reply 代理中继，实现请求的处理和转发功能
type reply struct {
	usedTotal   atomic.Int64
	usedSuccess atomic.Int64
}

func (hc *reply) getRetry(req *http.Request) int {
	str := req.Header.Get("X-Man-Retry")
	if str == "" {
		return getProxyRetry()
	}
	num, _ := strconv.Atoi(str)
	maxAllow := getProxyRetryMax()
	if num > 0 {
		return min(num, maxAllow)
	}
	return getProxyRetry()
}

// ServeHTTP 处理代理请求
func (hc *reply) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	hc.usedTotal.Add(1)
	ctx := req.Context()
	user := getProxyAuthorInfo(req)

	xlog.AddAttr(ctx, xlog.String("User", user.Name))

	if !checkHTTPAuth(user) {
		xlog.AddAttr(ctx, xlog.String("Error", "auth failed"), xlog.String("ErrPassword", user.Password))
		w.Header().Set("Proxy-Authenticate", "Basic realm=auth need")
		w.WriteHeader(407)
		w.Write([]byte("proxy auth failed"))
		return
	}

	req.Header.Del("Connection")

	for k := range req.Header {
		kl := strings.ToLower(k)
		if strings.HasPrefix(kl, "x-man") || strings.HasPrefix(kl, "proxy-") {
			req.Header.Del(k)
		}
	}
	if req.Body != nil {
		defer req.Body.Close()
	}

	if req.Method == http.MethodConnect {
		hc.handleConnect(w, req, user)
		return
	}

	hc.handleHTTP(w, req, user)
}

// handleHTTP  处理代理的 http 请求(非 https)
//
//	POST http://ifconfig.me/all.json HTTP/1.1
//
// Accept-Encoding: gzip
// Content-Length: 0
// Proxy-Authorization: Basic dW46cHN3
// User-Agent: Go-http-client/1.1
func (hc *reply) handleHTTP(w http.ResponseWriter, req *http.Request, user *User) {
	rr, err := http.NewRequestWithContext(req.Context(), req.Method, req.RequestURI, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(err.Error()))
		return
	}

	for k, v := range req.Header {
		for _, vv := range v {
			rr.Header.Add(k, vv)
		}
	}

	hc.forwardRequest(req.Context(), w, rr, user.Name)
}

// forwardRequest 转发代理请求，rr *http.Request 是要经过代理服务器的请求信息
func (hc *reply) forwardRequest(ctx context.Context, w http.ResponseWriter, rr *http.Request, username string) {
	p, err := pool.getOneProxyActive(username)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(err.Error()))
		return
	}

	p.State.UsedTotal.Add(1)

	client, err := httpClientProxied(p.Base.URL)
	if err != nil {
		xlog.Warn(ctx, "get transport failed", xlog.ErrorAttr("Error", err))
		return
	}

	resp, err := client.Do(rr)
	if err != nil {
		w.WriteHeader(550)
		xlog.Warn(ctx, "fetch response failed", xlog.ErrorAttr("Error", err))
		return
	}

	defer resp.Body.Close()

	resp.Header.Del("Connection")

	w.WriteHeader(resp.StatusCode)

	copyProxyResponseHeaders(w.Header(), resp.Header)
	io.Copy(w, resp.Body)
	p.State.UsedSuccess.Add(1)
}

func (hc *reply) getProxyServerConn(username string, req *http.Request) (*proxyEntry, net.Conn, error) {
	ctx := req.Context()
	attempt := hc.getRetry(req) + 1
	for i := 0; i < attempt; i++ {
		select {
		case <-ctx.Done():
			return nil, nil, context.Cause(ctx)
		default:
		}
		one, err := pool.getOneProxyActive(username)
		if err != nil {
			continue
		}

		// 每拿出来依次使用计数就+1，在交互成功后，给成功计数器+1
		one.State.UsedTotal.Add(1)

		tr, err := transport.Get(one.Base.URL)
		if err != nil {
			return one, nil, err
		}
		conn, err := tr.Connect(ctx, "tcp", req.RequestURI)
		if err == nil {
			return one, conn, nil
		}
	}
	return nil, nil, fmt.Errorf("failed to connect to proxy server (attempt %d)", attempt)
}

func (hc *reply) getClientConn(w http.ResponseWriter) (net.Conn, error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("%T not http.Hijacker", w)
	}
	conn, _, err := hj.Hijack()
	return conn, err
}

// handleConnect 处理 CONNECT 请求
func (hc *reply) handleConnect(w http.ResponseWriter, req *http.Request, user *User) {
	conn, err := hc.getClientConn(w)
	if err != nil {
		xlog.AddAttr(req.Context(), xlog.ErrorAttr("Error", err), xlog.String("Action", "getClientConn"))
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("can not hijack"))
		return
	}
	defer conn.Close()

	// CONNECT 请求的 RequestURI 就是目标地址 如 example.com:443
	one, sConn, err := hc.getProxyServerConn(user.Name, req)
	if err != nil {
		xlog.AddAttr(req.Context(), xlog.ErrorAttr("Error", err), xlog.String("Action", "getProxyServerConn"))
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("connect to proxy server failed:" + err.Error()))
		return
	}
	defer sConn.Close()

	_, err = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		xlog.AddAttr(req.Context(), xlog.ErrorAttr("Error", err), xlog.String("Action", "send Connection Established"))
		return
	}

	deadLine := time.Now().Add(getProxyTimeout())
	sConn.SetDeadline(deadLine)

	var closeOnce sync.Once
	doClose := func() {
		closeOnce.Do(func() {
			sConn.Close()
			conn.Close()
		})
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		n, e := io.Copy(conn, sConn)
		xlog.AddAttr(req.Context(), xlog.Int64("CopyToClientN", n), xlog.ErrorAttr("CopyToClientErr", e))
		doClose()
	})
	wg.Go(func() {
		n, e := io.Copy(sConn, conn)
		xlog.AddAttr(req.Context(), xlog.Int64("CopyToServerN", n), xlog.ErrorAttr("CopyToServerErr", e))
		doClose()
	})
	wg.Go(func() {
		<-req.Context().Done()
		doClose()
	})
	wg.Wait()
	one.State.UsedSuccess.Add(1)
}

func copyProxyResponseHeaders(dst, src http.Header) {
	for k, vs := range src {
		if strings.HasPrefix(strings.ToUpper(k), "proxy-") {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
