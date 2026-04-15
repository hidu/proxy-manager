package internal

import (
	"bytes"
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
	"github.com/xanygo/anygo/xvalidator"

	"github.com/hidu/proxy-manager/internal/transport"
)

var defaultRelay = &reply{}

// reply 代理中继，实现请求的处理和转发功能
type reply struct {
	usedTotal   atomic.Int64
	usedSuccess atomic.Int64
}

func getRetryWithRequest(req *http.Request) int {
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

	cleanProxyHeader(req.Header)
	if req.Body != nil {
		defer req.Body.Close()
	}

	if req.Method == http.MethodConnect {
		hc.handleConnect(w, req, user)
		return
	}

	hc.handleHTTP(w, req, user)
}

func cleanProxyHeader(hd http.Header) {
	hd.Del("Connection")
	for k := range hd {
		kl := strings.ToLower(k)
		if strings.HasPrefix(kl, "x-man-") || strings.HasPrefix(kl, "proxy-") {
			hd.Del(k)
		}
	}
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
	if err := xvalidator.IsHTTPURL(req.RequestURI); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("cannot read body:" + err.Error()))
		return
	}

	rr, err := http.NewRequestWithContext(req.Context(), req.Method, req.RequestURI, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(err.Error()))
		return
	}
	rr.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	// 在 handleHTTP 之前，已经清理了
	for k, v := range req.Header {
		for _, vv := range v {
			rr.Header.Add(k, vv)
		}
	}

	param := forwardParam{
		Request:  rr,
		Username: user.Name,
		Attempt:  getRetryWithRequest(req) + 1,
	}
	hc.forwardRequest(req.Context(), w, param)
}

type forwardParam struct {
	Request  *http.Request
	Username string
	Attempt  int
}

// forwardParam 转发代理请求，rr *http.Request 是要经过代理服务器的请求信息
func (hc *reply) forwardRequest(ctx context.Context, w http.ResponseWriter, rr forwardParam) {
	var p *proxyEntry
	var err error
	var resp *http.Response
	var i int
	for i = 0; i < rr.Attempt; i++ {
		hc.usedTotal.Add(1)
		p, err = pool.getOneProxyActive(ctx, rr.Username)
		if err != nil {
			xlog.Warn(ctx, "getOneProxyActive failed", xlog.ErrorAttr("Error", err))
			continue
		}

		p.State.UsedTotal.Add(1)

		client, err := httpClientProxied(p.Base.URL)
		if err != nil {
			xlog.Warn(ctx, "get transport failed", xlog.ErrorAttr("Error", err), xlog.String("Proxy", p.Base.Proxy))
			continue
		}

		resp, err = client.Do(rr.Request)
		if err != nil {
			xlog.Warn(ctx, "fetch response failed", xlog.ErrorAttr("Error", err))
			continue
		}
		break
	}

	defer resp.Body.Close()

	resp.Header.Del("Connection")

	copyProxyResponseHeaders(w.Header(), resp.Header)
	w.Header().Set("X-Man-Attempt", fmt.Sprintf("%d/%d", i, rr.Attempt))
	w.Header().Set("X-Man-Via", p.Base.URL.Hostname())

	w.WriteHeader(resp.StatusCode)

	io.Copy(w, resp.Body)
	p.State.UsedSuccess.Add(1)
	hc.usedSuccess.Add(1)
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
	if _, _, err := net.SplitHostPort(req.RequestURI); err != nil {
		xlog.AddAttr(req.Context(), xlog.ErrorAttr("Error", err), xlog.String("Action", "handleConnect"))
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("invalid connect request with uri: " + req.RequestURI))
		return
	}
	conn, err := hc.getClientConn(w)
	if err != nil {
		xlog.AddAttr(req.Context(), xlog.ErrorAttr("Error", err), xlog.String("Action", "getClientConn"))
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("can not hijack"))
		return
	}
	defer conn.Close()

	// CONNECT 请求的 RequestURI 就是目标地址 如 example.com:443
	one, sConn, err := hc.getProxyServerConn(req.Context(), user.Name, getRetryWithRequest(req)+1, req.RequestURI)
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

func (hc *reply) getProxyServerConn(ctx context.Context, username string, attempt int, targetAddr string) (*proxyEntry, net.Conn, error) {
	for i := 0; i < attempt; i++ {
		select {
		case <-ctx.Done():
			return nil, nil, context.Cause(ctx)
		default:
		}
		one, err := pool.getOneProxyActive(ctx, username)
		if err != nil {
			continue
		}

		// 每拿出来依次使用计数就+1，在交互成功后，给成功计数器+1
		one.State.UsedTotal.Add(1)

		tr, err := transport.Get(one.Base.URL)
		if err != nil {
			return one, nil, err
		}
		conn, err := tr.Connect(ctx, "tcp", targetAddr)
		if err == nil {
			return one, conn, nil
		}
	}
	return nil, nil, fmt.Errorf("failed to connect to proxy server (attempt %d)", attempt)
}

func copyProxyResponseHeaders(dst, src http.Header) {
	for k, vs := range src {
		if strings.HasPrefix(strings.ToUpper(k), "proxy-") {
			continue
		}
		switch k {
		case "Set-Cookie", "Content-Security-Policy", "Referrer-Policy":
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
