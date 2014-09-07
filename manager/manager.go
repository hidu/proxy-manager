package manager

import (
	"fmt"
	"github.com/hidu/goutils"
	"log"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"
)

type ProxyManager struct {
	httpProxy *HttpProxy
	config    *Config
	reqNum    int64
}

func NewProyManager() *ProxyManager {
	rand.Seed(time.Now().UnixNano())
	manager := &ProxyManager{}
	manager.httpProxy = NewHttpProxy(manager)
	manager.config = NewConfig()
	return manager
}

func (manager *ProxyManager) Start() {
	addr := fmt.Sprintf("%s:%d", "", manager.config.port)
	fmt.Println("start proxy manager at:", addr)
	err := http.ListenAndServe(addr, manager)
	log.Println(err)
}

func (manager *ProxyManager) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	host, port_int, err := utils.Net_getHostPortFromReq(req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
		log.Println("bad request,err", err)
		return
	}
	atomic.AddInt64(&manager.reqNum, 1)

	isLocalReq := port_int == manager.config.port
	if isLocalReq {
		isLocalReq = utils.Net_isLocalIp(host)
	}

	if isLocalReq {
		manager.serveLocalRequest(w, req)
	} else {
		manager.httpProxy.ServeHTTP(w, req)
	}
}

func (manager *ProxyManager) serveLocalRequest(w http.ResponseWriter, req *http.Request) {
	fmt.Fprint(w, "hello proxy manager")
}
