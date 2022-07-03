package internal

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"
)

type requestLog struct {
	data      []string
	logData   []string
	startTime time.Time
	req       *http.Request
	logID     int64
}

func newRequestLog(req *http.Request) *requestLog {
	rlog := &requestLog{req: req}
	rlog.reset()
	return rlog
}

func (rlog *requestLog) print() {
	if len(rlog.logData) == 0 {
		return
	}
	used := time.Now().Sub(rlog.startTime)
	log.Println("logID:", rlog.logID,
		rlog.req.Method, rlog.req.URL.String(),
		strings.Join(rlog.data, " "),
		strings.Join(rlog.logData, " "),
		"used:", used.String())
	rlog.reset()
}

func (rlog *requestLog) setLog(arg ...interface{}) {
	rlog.data = append(rlog.logData, fmt.Sprint(arg))
}
func (rlog *requestLog) addLog(arg ...interface{}) {
	rlog.logData = append(rlog.logData, fmt.Sprint(arg))
}
func (rlog *requestLog) reset() {
	rlog.startTime = time.Now()
	rlog.logData = []string{}
}

type httpClient struct {
	ProxyManager *ProxyManager
}

func newHTTPClient(manager *ProxyManager) *httpClient {
	log.Println("loading http client...")
	proxy := new(httpClient)
	proxy.ProxyManager = manager

	return proxy
}

func _reqFix(req *http.Request) {
	if req.Method == "CONNECT" && req.URL.Scheme == "" {
		req.URL.Scheme = "https"
	}
	if req.URL.Scheme == "" {
		req.URL.Scheme = "http"
	}
}

func (hc *httpClient) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	_reqFix(req)

	rlog := newRequestLog(req)

	rlog.logID = hc.ProxyManager.reqNum + time.Now().Unix()

	defer rlog.print()
	user := getProxyAuthorInfo(req)
	rlog.setLog("uname", user.Name)

	if ProxyDebug {
		dump, _ := httputil.DumpRequest(req, true)
		log.Println("request dump:\n", string(dump))
	}

	if !hc.ProxyManager.checkHTTPAuth(user) {
		rlog.addLog("auth", "failed")
		w.Header().Set("Proxy-Authenticate", "Basic realm=auth need")
		w.WriteHeader(407)
		w.Write([]byte("proxy auth failed"))
		return
	}

	req.RequestURI = ""
	req.Header.Del("Connection")
	req.Header.Del("Proxy-Connection")
	req.Header.Del("Proxy-Authorization")

	_statusOk := strings.Split(req.Header.Get("X-Man-Status-Ok"), ",")
	statusOk := make(map[int]int)
	for _, v := range _statusOk {
		_code, _ := strconv.Atoi(v)
		if _code > 0 {
			statusOk[_code] = 1
		}
	}
	_clientReTry := -1
	xManRetry := req.Header.Get("X-Man-ReTry")
	if xManRetry != "" {
		_clientReTry, _ = strconv.Atoi(xManRetry)
	}

	for k := range req.Header {
		k = strings.ToLower(k)
		if strings.HasPrefix(k, "x-man") || strings.HasPrefix(k, "proxy-") {
			req.Header.Del(k)
		}
	}
	if req.Body != nil {
		defer req.Body.Close()
	}

	var resp *http.Response
	var err error

	maxReTry := hc.ProxyManager.config.getReTry() + 1
	if _clientReTry >= 0 && _clientReTry <= hc.ProxyManager.config.ReTryMax {
		maxReTry = _clientReTry + 1
	}
	no := 1
	for ; no <= maxReTry; no++ {
		rlog.addLog("try", fmt.Sprintf("%d/%d", no, maxReTry))
		proxy, err := hc.ProxyManager.proxyPool.getOneProxy(user.Name)
		if err != nil {
			rlog.addLog("get_proxy_failed", err)
			rlog.print()
			break
		}
		rlog.addLog("proxy", proxy.proxy)
		rlog.addLog("proxyUsed", proxy.Used)
		client, err := newClient(proxy.URL, hc.ProxyManager.config.getTimeout())
		if err != nil {
			rlog.addLog("get http client failed", err)
			continue
		}
		resp, err = client.Do(req)
		if err == nil {
			if ProxyDebug {
				dump, _ := httputil.DumpResponse(resp, true)
				log.Println("response dump:\n", string(dump))
			}
			if hc.ProxyManager.config.IsWrongCode(resp.StatusCode) {
				rlog.addLog("statusCode wrong def in conf", resp.StatusCode)
				goto failed
			}
			if len(statusOk) != 0 {
				if _, has := statusOk[resp.StatusCode]; !has {
					rlog.addLog("statusCode wrong", resp.StatusCode)
					goto failed
				}
			}
			hc.ProxyManager.proxyPool.markProxyStatus(proxy, proxyUsedSuc)
			break
		} else {
			rlog.addLog("response error", err.Error())
		}

	failed:
		{
			hc.ProxyManager.proxyPool.markProxyStatus(proxy, proxyUsedFailed)
			rlog.addLog("failed")
			if no == maxReTry {
				rlog.addLog("all failed")
			}
			rlog.print()
		}
	}

	w.Header().Set("x-man-try", fmt.Sprintf("%d/%d", no, maxReTry))
	w.Header().Set("x-man-id", fmt.Sprintf("%d", rlog.logID))

	if err != nil || resp == nil {
		w.WriteHeader(550)
		w.Write([]byte("all failed," + fmt.Sprintf("try:%d", no)))
		return
	}
	// log.Println("resp.Header:", resp.Header)

	resp.Header.Del("Content-Length")
	resp.Header.Del("Connection")

	w.WriteHeader(resp.StatusCode)
	rlog.addLog("status:", resp.StatusCode)

	copyHeaders(w.Header(), resp.Header)
	n, err := io.Copy(w, resp.Body)
	rlog.addLog("res_len:", n)
	if err != nil {
		// client may be not read the body
		rlog.addLog("io.copy_err:", err)
	}

	if err := resp.Body.Close(); err != nil {
		rlog.addLog("close response body err:", err)
	}
	rlog.addLog("OK")
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		if len(k) > 5 && k[:6] == "Proxy-" {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}