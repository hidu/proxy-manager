package manager

import (
	"bytes"
	"fmt"
	"github.com/hidu/goutils"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"
)

const cookieName = "x-man-proxy"

type webRequestCtx struct {
	values  map[string]interface{}
	user    *user
	isLogin bool
}

func (ctx *webRequestCtx) isAdmin() bool {
	return ctx.isLogin && ctx != nil && ctx.user.IsAdmin
}

func (manager *ProxyManager) serveLocalRequest(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	defer (func() {
		log.Println(req.RemoteAddr, req.RequestURI, "used:", time.Now().Sub(start))
	})()

	if strings.HasPrefix(req.URL.Path, "/res/") {
		Assest.HTTPHandler("/").ServeHTTP(w, req)
		return
	}

	user, isLogin := manager.handelCheckLogin(req)

	values := make(map[string]interface{})
	values["title"] = manager.config.title
	values["subTitle"] = ""
	values["isLogin"] = isLogin

	if isLogin {
		values["uname"] = user.Name
		values["isAdmin"] = user.IsAdmin
	} else {
		values["uname"] = ""
		values["isAdmin"] = false
	}

	values["startTime"] = manager.startTime.Format(timeFormatStd)
	values["version"] = GetVersion()
	values["notice"] = manager.config.notice
	values["port"] = fmt.Sprintf("%d", manager.config.port)
	values["config"] = manager.config

	values["proxyReqTotal"] = manager.proxyPool.Count.Total

	_host, _port, _ := utils.Net_getHostPortFromReq(req)
	values["proxy_host"] = _host
	values["proxy_port"] = _port

	ctx := &webRequestCtx{
		values:  values,
		user:    user,
		isLogin: isLogin,
	}

	funcMap := make(map[string]func(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx))

	funcMap["/"] = manager.handelIndex
	funcMap["/about"] = manager.handelAbout
	funcMap["/add"] = manager.handelAdd
	funcMap["/test"] = manager.handelTest
	funcMap["/login"] = manager.handelLogin
	funcMap["/logout"] = manager.handelLogout

	if fn, has := funcMap[req.URL.Path]; has {
		fn(w, req, ctx)
	} else {
		http.NotFound(w, req)
	}

}

func (manager *ProxyManager) handelIndex(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	values := ctx.values
	values["proxy_count_suc"] = manager.proxyPool.Count.Success
	values["proxy_count_failed"] = manager.proxyPool.Count.Failed
	values["proxy_count"] = manager.proxyPool.GetProxyNums()
	values["proxys"] = manager.proxyPool.proxyListActive

	code := renderHTML("index.html", values, true)
	w.Write([]byte(code))
}
func (manager *ProxyManager) handelAdd(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	values := ctx.values
	doPost := func() {
		if !ctx.isAdmin() {
			w.Write([]byte("<script>alert('must admin');</script>"))
			log.Println("no admin", req.RemoteAddr)
			return
		}

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
		code := renderHTML("add.html", values, true)
		w.Write([]byte(code))
		return
	case "POST":
		doPost()
		return

	}
	http.NotFound(w, req)
}
func (manager *ProxyManager) handelAbout(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	values := ctx.values
	code := renderHTML("about.html", values, true)
	w.Write([]byte(code))
}
func (manager *ProxyManager) handelLogout(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	cookie := &http.Cookie{Name: cookieName, Value: "", Path: "/"}
	http.SetCookie(w, cookie)
	http.Redirect(w, req, "/", 302)
}
func (manager *ProxyManager) handelTest(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	values := ctx.values
	doPost := func() {
		token, err := strconv.ParseInt(req.PostFormValue("token"), 10, 64)
		if err != nil {
			w.Write([]byte("params wrong"))
			return
		}
		urlStr := strings.TrimSpace(req.PostFormValue(fmt.Sprintf("url_%d", token-manager.startTime.UnixNano())))
		obj, err := url.Parse(urlStr)
		if err != nil || obj.Scheme != "http" {
			w.Write([]byte(fmt.Sprintf("wrong url [%s],err:%v", urlStr, err)))
			return
		}
		proxyStr := strings.TrimSpace(req.PostFormValue("proxy"))

		if proxyStr != "" {
			_, err := url.Parse(proxyStr)
			if err != nil {
				w.Write([]byte(fmt.Sprintf("wrong proxy info [%s],err:%v", proxyStr, err)))
				return
			}
			proxy := newProxy(proxyStr)
			resp, err := doRequestGet(urlStr, proxy, 5)
			if err != nil {
				w.WriteHeader(502)
				w.Write([]byte(fmt.Sprintf("can not get [%s] via [%s]\nerr:%s", urlStr, proxyStr, err)))
				return
			}
			copyHeaders(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
			resp.Body.Close()

		} else {
			reqTest, _ := http.NewRequest("GET", urlStr, nil)
			reqTest.SetBasicAuth(defaultTestUser.Name, defaultTestUser.Psw)
			reqTest.Header.Set(proxyAuthorizatonHeader, reqTest.Header.Get("Authorization"))
			reqTest.Header.Del("Authorization")

			manager.httpClient.ServeHTTP(w, reqTest)
		}

	}

	switch req.Method {
	case "GET":
		nowInt := time.Now().UnixNano()
		values["url_name"] = fmt.Sprintf("url_%d", nowInt)

		values["token"] = fmt.Sprintf("%d", manager.startTime.UnixNano()+nowInt)

		code := renderHTML("test.html", values, true)
		w.Write([]byte(code))
		return
	case "POST":
		doPost()
		return

	}
	http.NotFound(w, req)
}

func (manager *ProxyManager) handelLogin(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	values := ctx.values
	if req.Method == "POST" {
		name := req.PostFormValue("name")
		psw := req.PostFormValue("psw")
		if user, has := manager.users[name]; has && user.pswEq(psw) {
			log.Println("login suc,name=", name)
			cookie := &http.Cookie{
				Name:    cookieName,
				Value:   fmt.Sprintf("%s:%s", name, user.PswEnc()),
				Path:    "/",
				Expires: time.Now().Add(86400 * time.Second),
			}
			http.SetCookie(w, cookie)
			w.Write([]byte("<script>parent.location.href='/'</script>"))
		} else {
			w.Write([]byte("<script>alert('login failed')</script>"))
		}
	} else {
		code := renderHTML("login.html", values, true)
		w.Write([]byte(code))
	}
}

func (manager *ProxyManager) handelCheckLogin(req *http.Request) (user *user, isLogin bool) {
	if req == nil {
		return
	}
	cookie, err := req.Cookie(cookieName)
	if err != nil {
		return
	}
	info := strings.SplitN(cookie.Value, ":", 2)
	if len(info) != 2 {
		return
	}
	if user, has := manager.users[info[0]]; has && user.PswEnc() == info[1] {
		return user, true
	}
	return
}

func renderHTML(fileName string, values map[string]interface{}, layout bool) string {
	//	html := resource.DefaultResource.Load("/res/tpl/" + fileName)
	html := Assest.GetContent("/res/tpl/" + fileName)
	myfn := template.FuncMap{
		"shortTime": func(tu int64) string {
			t := time.Unix(tu, 0)
			return t.Format(timeFormatStd)
		},
		"myNum": func(n int64) string {
			if n == 0 {
				return ""
			}
			return fmt.Sprintf("%d", n)
		},
	}

	tpl, _ := template.New("page").Funcs(myfn).Parse(string(html))

	var bf []byte
	w := bytes.NewBuffer(bf)
	tpl.Execute(w, values)
	body := w.String()
	if layout {
		values["body"] = body
		return renderHTML("layout.html", values, false)
	}
	return utils.Html_reduceSpace(body)
}
