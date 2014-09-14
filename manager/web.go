package manager

import (
	"bytes"
	"fmt"
	"github.com/hidu/goutils"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"
)

func (manager *ProxyManager) serveLocalRequest(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	defer (func() {
		log.Println(req.RemoteAddr, req.RequestURI, "used:", time.Now().Sub(start))
	})()

	if strings.HasPrefix(req.URL.Path, "/res/") {
		utils.DefaultResource.HandleStatic(w, req, req.URL.Path)
		return
	}

	values := make(map[string]interface{})
	values["title"] = manager.config.title
	values["subTitle"] = ""
	values["startTime"] = manager.startTime.Format(TIME_FORMAT_STD)
	values["version"] = ProxyVersion
	values["notice"] = manager.config.notice
	values["port"] = fmt.Sprintf("%d", manager.config.port)
	values["config"] = manager.config

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
	values["proxy_count_suc"] = manager.proxyPool.Count.success
	values["proxy_count_failed"] = manager.proxyPool.Count.failed
	code := render_html("index.html", values, true)
	w.Write([]byte(code))
}
func (manager *ProxyManager) handel_add(w http.ResponseWriter, req *http.Request, values map[string]interface{}) {
	do_post := func() {
		proxysTxt := req.PostFormValue("proxys")
		txtFile := utils.NewTxtFileFromString(proxysTxt)

		proxys, _ := manager.proxyPool.loadProxysFromTxtFile(txtFile)
		if len(proxys) == 0 {
			w.Write([]byte("<script>alert('no proxy');</script>"))
			log.Println("no proxy,form input:[", proxysTxt, "]")
			return
		}
		n := 0
		for _, proxy := range proxys {
			if manager.proxyPool.addProxy(proxy) {
				n++
			}
		}
		if n > 0 {
			go manager.proxyPool.runTest()
		}
		w.Write([]byte(fmt.Sprintf("<script>alert('add %d new proxy');</script>", n)))
	}

	switch req.Method {
	case "GET":
		code := render_html("add.html", values, true)
		w.Write([]byte(code))
		return
	case "POST":
		do_post()
		return

	}
	http.NotFound(w, req)
}
func (manager *ProxyManager) handel_about(w http.ResponseWriter, req *http.Request, values map[string]interface{}) {
	code := render_html("about.html", values, true)
	w.Write([]byte(code))
}
func (manager *ProxyManager) handel_test(w http.ResponseWriter, req *http.Request, values map[string]interface{}) {
	do_post := func() {
		urlStr := strings.TrimSpace(req.PostFormValue("url"))
		obj, err := url.Parse(urlStr)
		if err != nil || obj.Scheme != "http" {
			w.Write([]byte("wrong url"))
			return
		}
		proxyStr := strings.TrimSpace(req.PostFormValue("proxy"))

		if proxyStr != "" {
			proxyObj, err := url.Parse(proxyStr)
			if err != nil || proxyObj.Scheme != "http" {
				w.Write([]byte("wrong proxy info"))
				return
			}
			proxy := NewProxy(proxyStr)
			resp, err := doRequestGet(urlStr, proxy, 5)
			if err != nil {
				w.WriteHeader(502)
				w.Write([]byte(fmt.Sprintf("can not get %s via %s", urlStr, proxyStr)))
				return
			}
			copyHeaders(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
			resp.Body.Close()

		} else {
			reqTest, _ := http.NewRequest("GET", urlStr, nil)
			manager.httpClient.ServeHTTP(w, reqTest)
		}

	}
	switch req.Method {
	case "GET":
		code := render_html("test.html", values, true)
		w.Write([]byte(code))
		return
	case "POST":
		do_post()
		return

	}
	http.NotFound(w, req)
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
