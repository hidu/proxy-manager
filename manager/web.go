package manager

import (
	"bytes"
	"fmt"
	"github.com/hidu/goutils"
	"log"
	"net/http"
	"strings"
	"text/template"
)

func (manager *ProxyManager) serveLocalRequest(w http.ResponseWriter, req *http.Request) {
	log.Println(req.RemoteAddr, req.RequestURI, req.URL.RawQuery)

	if strings.HasPrefix(req.URL.Path, "/res/") {
		utils.DefaultResource.HandleStatic(w, req, req.URL.Path)
		return
	}

	values := make(map[string]interface{})
	values["title"] = manager.config.title
	values["subTitle"] = ""
	values["version"] = ProxyVersion
	values["notice"] = manager.config.notice
	values["port"] = fmt.Sprintf("%d", manager.config.port)

	values["proxyTotal"] = len(manager.proxyPool.proxyListActive)
	values["proxyAllNum"] = len(manager.proxyPool.proxyListAll)
	values["proxyReqTotal"] = manager.proxyPool.Count.total

	_host, _port, _ := utils.Net_getHostPortFromReq(req)
	values["proxy_host"] = _host
	values["proxy_port"] = _port

	funcMap := make(map[string]func(w http.ResponseWriter, req *http.Request, values map[string]interface{}))

	funcMap["/"] = manager.handel_index
	funcMap["/about"] = manager.handel_about
	funcMap["/add"] = manager.handel_add
	funcMap["/test"] = manager.handel_test

	if fn, has := funcMap[req.URL.Path]; has {
		fn(w, req, values)
	} else {
		http.NotFound(w, req)
	}

}

func (manager *ProxyManager) handel_index(w http.ResponseWriter, req *http.Request, values map[string]interface{}) {
	code := render_html("index.html", values, true)
	w.Write([]byte(code))
}
func (manager *ProxyManager) handel_add(w http.ResponseWriter, req *http.Request, values map[string]interface{}) {
	code := render_html("add.html", values, true)
	w.Write([]byte(code))
}
func (manager *ProxyManager) handel_about(w http.ResponseWriter, req *http.Request, values map[string]interface{}) {
	code := render_html("about.html", values, true)
	w.Write([]byte(code))
}
func (manager *ProxyManager) handel_test(w http.ResponseWriter, req *http.Request, values map[string]interface{}) {
	code := render_html("test.html", values, true)
	w.Write([]byte(code))
}

func render_html(fileName string, values map[string]interface{}, layout bool) string {
	html := utils.DefaultResource.Load("/res/tpl/" + fileName)
	tpl, _ := template.New("page").Parse(string(html))
	var bf []byte
	w := bytes.NewBuffer(bf)
	tpl.Execute(w, values)
	body := w.String()
	if layout {
		values["body"] = body
		return render_html("layout.html", values, false)
	}
	return utils.Html_reduceSpace(body)
}
