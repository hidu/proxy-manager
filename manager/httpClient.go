package manager

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

type requestLog struct {
	data      []string
	logData   []string
	startTime time.Time
	req       *http.Request
	logId     int64
}

func NewRequestLog(req *http.Request) *requestLog {
	rlog := &requestLog{req: req}
	rlog.reset()
	return rlog
}

func (rlog *requestLog) print() {
	if len(rlog.logData) == 0 {
		return
	}
	used := time.Now().Sub(rlog.startTime)
	log.Println("logid:", rlog.logId,
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

type HttpClient struct {
	ProxyManager *ProxyManager
}

func NewHttpClient(manager *ProxyManager) *HttpClient {
	log.Println("loading http client...")
	proxy := new(HttpClient)
	proxy.ProxyManager = manager

	return proxy
}

func (httpClient *HttpClient) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rlog := NewRequestLog(req)

	rlog.logId = httpClient.ProxyManager.reqNum + time.Now().Unix()

	defer rlog.print()
	user := getAuthorInfo(req)
	rlog.setLog("uname", user.Name)

	if PROXY_DEBUG {
		dump, _ := httputil.DumpRequest(req, true)
		log.Println("req dump:\n", string(dump))
	}

	if !httpClient.ProxyManager.checkHttpAuth(user) {
		rlog.addLog("auth", "failed")
		w.Header().Set("Proxy-Authenticate", "Basic realm=auth need")
		w.WriteHeader(407)
		w.Write([]byte("auth failed"))
		return
	}

	req.RequestURI = ""
	req.Header.Del("Connection")
	req.Header.Del("Proxy-Connection")

	_statusOk := strings.Split(req.Header.Get("X-Man-Status-Ok"), ",")
	statusOk := make(map[int]int)
	for _, v := range _statusOk {
		_code := int(getInt64(v))
		if _code > 0 {
			statusOk[_code] = 1
		}
	}
	_clientReTry := -1
	x_man_retry := req.Header.Get("X-Man-ReTry")
	if x_man_retry != "" {
		_clientReTry = int(getInt64(x_man_retry))
	}

	for k := range req.Header {
		k = strings.ToLower(k)
		if strings.HasPrefix(k, "x-man") || strings.HasPrefix(k, "proxy-") {
			req.Header.Del(k)
		}
	}

	var resp *http.Response
	var err error

	max_re_try := httpClient.ProxyManager.config.reTry + 1
	if _clientReTry >= 0 && _clientReTry <= httpClient.ProxyManager.config.reTryMax {
		max_re_try = _clientReTry + 1
	}
	no := 1
	for ; no <= max_re_try; no++ {
		rlog.addLog("try", fmt.Sprintf("%d/%d", no, max_re_try))
		proxy, err := httpClient.ProxyManager.proxyPool.GetOneProxy(user.Name)
		if err != nil {
			rlog.addLog("get_proxy_faield", err)
			rlog.print()
			break
		}
		rlog.addLog("proxy", proxy.proxy)
		rlog.addLog("proxyUsed", proxy.Used)
		client, err := NewClient(proxy.URL, httpClient.ProxyManager.config.timeout)
		if err != nil {
			rlog.addLog("get http client failed", err)
			continue
		}
		resp, err = client.Do(req)
		if err == nil {
			if _, has := httpClient.ProxyManager.config.wrongStatusCode[resp.StatusCode]; has {
				rlog.addLog("statusCode wrong def in conf", resp.StatusCode)
				goto failed
			}
			if len(statusOk) != 0 {
				if _, has := statusOk[resp.StatusCode]; !has {
					rlog.addLog("statusCode wrong", resp.StatusCode)
					goto failed
				}
			}
			httpClient.ProxyManager.proxyPool.MarkProxyStatus(proxy, PROXY_USED_SUC)
			break
		} else {
			rlog.addLog("resErr", err.Error())
		}

	failed:
		{
			httpClient.ProxyManager.proxyPool.MarkProxyStatus(proxy, PROXY_USED_FAILED)
			rlog.addLog("failed")
			if no == max_re_try {
				rlog.addLog("all failed")
			}
			rlog.print()
		}
	}

	w.Header().Set("x-man-try", fmt.Sprintf("%d/%d", no, max_re_try))
	w.Header().Set("x-man-id", fmt.Sprintf("%d", rlog.logId))

	if err != nil || resp == nil {
		w.WriteHeader(550)
		w.Write([]byte("all failed," + fmt.Sprintf("try:%d", no)))
		return
	}

	resp.Header.Del("Content-Length")
	resp.Header.Del("Connection")

	copyHeaders(w.Header(), resp.Header)
	rlog.addLog("status:", resp.StatusCode)

	w.WriteHeader(resp.StatusCode)

	n, err := io.Copy(w, resp.Body)
	rlog.addLog("res_len:", n)
	if err != nil {
		//client may be not read the body
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
