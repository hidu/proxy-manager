package internal

import (
	"log"
	"net/http"
	"strings"

	"github.com/xanygo/anygo/xattr"
	"github.com/xanygo/anygo/xhttp"
	"github.com/xanygo/anygo/xhttp/xhandler"
	"github.com/xanygo/anygo/xlog"
)

type Manager struct{}

func NewManager() *Manager {
	log.Println("starting...")
	pool = loadPool()
	manager := &Manager{}
	return manager
}

func (man *Manager) Start() {
	listen := xattr.AppMain().MustGetListen("main")
	log.Println("start proxy manager at:", listen)

	router := xhttp.NewRouter()
	router.Use((&xhandler.AccessLog{
		Logger: xlog.AccessLogger(),
	}).Next)
	router.Handle("*", man)

	err := http.ListenAndServe(listen, router)
	log.Println("proxy server exit:", err)
}

var web = &adminWeb{}

func (man *Manager) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// bf, _ := httputil.DumpRequest(req, false)
	// log.Println("ServeHTTP", req.Method, req.RequestURI, "request:\n", string(bf))

	if strings.EqualFold(req.Method, http.MethodConnect) || strings.HasPrefix(req.RequestURI, "http://") {
		defaultRelay.ServeHTTP(w, req)
		return
	}
	web.serveAdminPage(w, req)
}
