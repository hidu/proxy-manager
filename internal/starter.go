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

func Start() {
	log.Println("starting...")
	listen := xattr.AppMain().MustGetListen("main")
	log.Println("start proxy manager at:", listen)

	router := xhttp.NewRouter()
	router.Use((&xhandler.AccessLog{
		Logger: xlog.AccessLogger(),
	}).Next)

	router.Handle("*", &gateway{})

	err := http.ListenAndServe(listen, router)
	log.Println("proxy server exit:", err)
}

type gateway struct{}

func (gw *gateway) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// bf, _ := httputil.DumpRequest(req, false)
	// log.Println("ServeHTTP", req.Method, req.RequestURI, "request:\n", string(bf))

	if strings.EqualFold(req.Method, http.MethodConnect) || strings.HasPrefix(req.RequestURI, "http://") {
		defaultRelay.ServeHTTP(w, req)
		return
	}
	web.serveAdminPage(w, req)
}
