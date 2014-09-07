package manager

import (
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type HttpClient struct {
	ProxyManager *ProxyManager
}

func NewHttpClient(manager *ProxyManager) *HttpClient {
	proxy := new(HttpClient)
	proxy.ProxyManager = manager

	return proxy
}

func (httpClient *HttpClient) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	proxy, err := httpClient.ProxyManager.proxyPool.GetOneProxy()
	if err != nil {
		log.Println(err)
		w.WriteHeader(550)
		w.Write([]byte(err.Error()))
		return
	}
	proxyGetFn := func(req *http.Request) (*url.URL, error) {
		return proxy.urlObj, nil
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: proxyGetFn,
		},
	}
	log.Println("using proxy:", proxy.proxyUrl)
	dump, _ := httputil.DumpRequest(req, true)

	req.RequestURI = ""
	req.Header.Del("Connection")
	req.Header.Del("Proxy-Connection")
	log.Println("req dump:\n", string(dump))

	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		w.WriteHeader(551)
		w.Write([]byte(err.Error()))
		return
	}
	w.WriteHeader(resp.StatusCode)
	for k, v := range resp.Header {
		w.Header()[k] = v
	}

	dumpResp, _ := httputil.DumpResponse(resp, false)
	log.Println("resp:\n", string(dumpResp))

	io.Copy(w, resp.Body)
	if err := resp.Body.Close(); err != nil {
		log.Println("Can't close response body %v", err)
	}
}
