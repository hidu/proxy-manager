package manager

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

type requestLog struct {
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
		strings.Join(rlog.logData, " "),
		"used:", used.String())
	rlog.reset()
}

func (rlog *requestLog) addLog(arg ...interface{}) {
	rlog.logData = append(rlog.logData, fmt.Sprint(arg))
}
func (rlog *requestLog) reset() {
	rlog.startTime = time.Now()
	rlog.logData = []string{}
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

	defer httpClient.ProxyManager.proxyPool.CleanSessionProxy(rlog.logId)

	defer rlog.print()
	if PROXY_DEBUG {
		dump, _ := httputil.DumpRequest(req, true)
		log.Println("req dump:\n", string(dump))
	}
	req.RequestURI = ""
	req.Header.Del("Connection")
	req.Header.Del("Proxy-Connection")

	var resp *http.Response
	var err error

	client := &http.Client{}
	max_re_try := httpClient.ProxyManager.config.re_try + 1
	no := 0
	for ; no < max_re_try; no++ {
		rlog.addLog("try_no", no)
		proxy, err := httpClient.ProxyManager.proxyPool.GetOneProxy(rlog.logId)
		if err != nil {
			rlog.addLog("get_proxy_faield", err)
			rlog.print()
			break
		}
		rlog.addLog("proxy", proxy.proxy)
		rlog.addLog("proxyUsed", proxy.Used)
		proxyGetFn := func(req *http.Request) (*url.URL, error) {
			return proxy.URL, nil
		}

		client.Transport = &http.Transport{Proxy: proxyGetFn}
		resp, err = client.Do(req)
		if err == nil {
			break
		} else {
			rlog.addLog("failed")
			if no == max_re_try-1 {
				rlog.addLog("all failed")
			}
			rlog.print()
		}
	}

	w.Header().Set("x-man-try", fmt.Sprintf("%d", no))
	w.Header().Set("x-man-try-max", fmt.Sprintf("%d", max_re_try))
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
		rlog.addLog("io.copy_err:", err)
	}
	if err := resp.Body.Close(); err != nil {
		rlog.addLog("close response body err:", err)
	}
	rlog.addLog("OK")
}

func copyHeaders(dst, src http.Header) {
	for k := range dst {
		dst.Del(k)
	}
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
