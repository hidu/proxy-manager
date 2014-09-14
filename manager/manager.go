package manager

import (
	"fmt"
	"github.com/hidu/goutils"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

var PROXY_DEBUG bool = false

type ProxyManager struct {
	httpClient *HttpClient
	config     *Config
	proxyPool  *ProxyPool
	reqNum     int64
}

func NewProyManager(configPath string) *ProxyManager {
	log.Println("loading...")
	rand.Seed(time.Now().UnixNano())
	manager := &ProxyManager{}
	manager.config = LoadConfig(configPath)

	if manager.config == nil {
		os.Exit(1)
	}

	manager.proxyPool = LoadProxyPool(manager)
	if manager.proxyPool == nil {
		os.Exit(1)
	}

	manager.httpClient = NewHttpClient(manager)
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
		manager.httpClient.ServeHTTP(w, req)
	}
}
