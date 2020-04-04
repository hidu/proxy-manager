package internal

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/hidu/goutils/fs"
	"github.com/hidu/goutils/net_util"
	"github.com/hidu/goutils/time_util"
)

// ProxyDebug 是否debug
var ProxyDebug = false

// ProxyManager manager server
type ProxyManager struct {
	httpClient *httpClient
	config     *config
	proxyPool  *ProxyPool
	reqNum     int64
	startTime  time.Time
	users      map[string]*user
}

// NewProyManager init server
func NewProyManager(configPath string) *ProxyManager {
	log.Println("loading...")
	rand.Seed(time.Now().UnixNano())
	manager := &ProxyManager{}
	manager.startTime = time.Now()
	manager.reqNum = 0
	manager.config = loadConfig(configPath)

	if manager.config == nil {
		log.Println("parse config failed")
		os.Exit(1)
	}
	setupLog(fmt.Sprintf("%s/%d.log", manager.config.confDir, manager.config.port))

	manager.proxyPool = loadProxyPool(manager)
	if manager.proxyPool == nil {
		log.Println("parse proxyPool failed")
		os.Exit(1)
	}

	manager.loadUsers()

	time_util.SetInterval(func() {
		manager.loadUsers()
	}, 300)

	manager.httpClient = newHTTPClient(manager)
	return manager
}

// Start start server
func (manager *ProxyManager) Start() {
	addr := fmt.Sprintf("%s:%d", "", manager.config.port)
	fmt.Println("start proxy manager at:", addr)
	err := http.ListenAndServe(addr, manager)
	log.Println(err)
}

// ServeHTTP ServeHTTP
func (manager *ProxyManager) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	host, portInt, err := utils.Net_getHostPortFromReq(req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
		log.Println("bad request,err", err)
		return
	}
	// 	atomic.AddInt64(&(manager.reqNum), 1)

	isLocalReq := portInt == manager.config.port

	if isLocalReq {
		isLocalReq = utils.Net_isLocalIp(host)
	}

	if isLocalReq {
		manager.serveLocalRequest(w, req)
	} else {
		manager.httpClient.ServeHTTP(w, req)
	}
}

func (manager *ProxyManager) loadUsers() {
	var err error
	manager.users, err = loadUsers(manager.config.confDir + "/users")
	if err != nil {
		log.Println("loadUsers err:", err)
	} else {
		log.Println("loadUsers suc,total:", len(manager.users))
	}

}

func setupLog(logPath string) {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		log.Println("create log file failed [", logPath, "]", err)
		os.Exit(2)
	}
	log.SetOutput(logFile)

	time_util.SetInterval(func() {
		if !fs.FileExists(logPath) {
			logFile.Close()
			logFile, _ = os.OpenFile(logPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
			log.SetOutput(logFile)
		}
	}, 30)
}
