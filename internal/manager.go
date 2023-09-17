package internal

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsgo/fsenv"
	"github.com/fsgo/fsgo/fsfs"
)

// ProxyDebug 是否debug
var ProxyDebug = os.Getenv("ProxyManagerDebug") == "true"

// ProxyManager manager server
type ProxyManager struct {
	startTime  time.Time
	httpClient *httpClient
	config     *Config
	proxyPool  *ProxyPool
	users      map[string]*User
	reqNum     int64
	mux        sync.RWMutex
}

// NewProxyManager init server
func NewProxyManager(configPath string) *ProxyManager {
	log.Println("starting...")
	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalln("parse Config failed:", err)
	}

	setEnv(configPath)
	setupLog(filepath.Join(fsenv.LogRootDir(), "proxy.log"))

	manager := &ProxyManager{
		startTime: time.Now(),
		config:    cfg,
	}
	manager.proxyPool = loadPool(cfg)

	manager.loadUsers()
	SetInterval(func() {
		manager.loadUsers()
	}, 300*time.Second)

	manager.httpClient = newHTTPClient(manager)
	return manager
}

func setEnv(cfp string) {
	abs, err := filepath.Abs(cfp)
	if err != nil {
		log.Fatalln(err)
	}
	confDir := filepath.Dir(abs)
	fsenv.SetConfRootDir(confDir)
	rootDir := filepath.Dir(confDir)
	fsenv.SetRootDir(rootDir)
}

// Start start server
func (man *ProxyManager) Start() {
	addr := fmt.Sprintf("%s:%d", "", man.config.Port)
	log.Println("start proxy manager at:", addr)
	err := http.ListenAndServe(addr, man)
	log.Println("proxy server exit:", err)
}

// ServeHTTP ServeHTTP
func (man *ProxyManager) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	host, portInt, err := getHostPortFromReq(req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
		log.Println("bad request, err:", err)
		return
	}

	isLocalReq := portInt == man.config.Port

	if isLocalReq {
		isLocalReq = isLocalIP(host)
	}

	if isLocalReq {
		man.serveLocalRequest(w, req)
	} else {
		man.httpClient.ServeHTTP(w, req)
	}
}

func (man *ProxyManager) loadUsers() {
	users, err := loadUsers(filepath.Join(fsenv.ConfRootDir(), "users.toml"))
	if err != nil {
		log.Println("loadUsers err:", err)
		return
	}
	log.Println("loadUsers success, total:", len(users))
	man.mux.Lock()
	man.users = users
	man.mux.Unlock()
}

func (man *ProxyManager) getUser(name string) *User {
	man.mux.RLock()
	defer man.mux.RUnlock()
	if len(man.users) == 0 {
		return nil
	}
	return man.users[name]
}

func setupLog(logPath string) {
	f := &fsfs.Rotator{
		Path:    logPath,
		ExtRule: "1hour",
	}
	defer f.Close()
	if err := f.Init(); err != nil {
		log.Fatalln("setup logfile failed, path=", logPath, "err=", err)
	}
	log.Println("setup logfile with", logPath)
	mw := io.MultiWriter(os.Stderr, f)
	log.SetOutput(mw)
}
