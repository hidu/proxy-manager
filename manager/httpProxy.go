package manager

import (
	"github.com/hidu/goproxy"
	"net/http"
)

type HttpProxy struct {
	GoProxy *goproxy.ProxyHttpServer
	ProxyManager *ProxyManager
}

func NewHttpProxy(manager *ProxyManager) *HttpProxy {
	proxy := new(HttpProxy)
	proxy.ProxyManager = manager
	proxy.GoProxy = goproxy.NewProxyHttpServer()
	return proxy
}

func (httpProxy *HttpProxy)ServeHTTP(w http.ResponseWriter, req *http.Request){
  httpProxy.GoProxy.ServeHTTP(w,req)
}