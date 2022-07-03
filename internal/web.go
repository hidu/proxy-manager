package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/hidu/goutils/str_util"
)

const cookieName = "x-man-proxy"

type webRequestCtx struct {
	values  map[string]interface{}
	user    *User
	isLogin bool
	req     *http.Request
	start   time.Time
	logMsg  string
}

func (ctx *webRequestCtx) isAdmin() bool {
	return ctx.isLogin && ctx != nil && ctx.user.Admin
}

func (ctx *webRequestCtx) finalLog() {
	req := ctx.req
	log.Println(req.RemoteAddr, req.Method, req.RequestURI, "used:", time.Now().Sub(ctx.start), "refer:", req.Referer(), "ua:", req.UserAgent(), "logMsg:", ctx.logMsg)
}

func (ctx *webRequestCtx) addLogMsg(msg ...interface{}) {
	if ctx.logMsg != "" {
		ctx.logMsg += ","
	}
	ctx.logMsg += fmt.Sprint(msg...)
}

func (man *ProxyManager) serveLocalRequest(w http.ResponseWriter, req *http.Request) {
	ctx := &webRequestCtx{
		req:   req,
		start: time.Now(),
	}
	defer ctx.finalLog()

	if strings.HasPrefix(req.URL.Path, "/asset/") {
		http.FileServer(http.FS(files)).ServeHTTP(w, req)
		return
	}

	user, isLogin := man.handelCheckLogin(req, ctx)

	values := make(map[string]interface{})
	values["title"] = man.config.Title
	values["subTitle"] = ""
	values["isLogin"] = isLogin

	if isLogin {
		values["uname"] = user.Name
		values["isAdmin"] = user.Admin
	} else {
		values["uname"] = ""
		values["isAdmin"] = false
	}

	values["startTime"] = man.startTime.Format(timeFormatStd)
	values["version"] = version
	values["notice"] = man.config.Notice
	values["port"] = strconv.Itoa(man.config.Port)
	values["Config"] = man.config

	values["proxyReqTotal"] = man.proxyPool.Count.Total

	_host, _port, _ := getHostPortFromReq(req)
	values["proxy_host"] = _host
	values["proxy_port"] = _port

	ctx.values = values
	ctx.user = user
	ctx.isLogin = isLogin

	funcMap := make(map[string]func(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx))

	funcMap["/"] = man.handelIndex
	funcMap["/about"] = man.handelAbout
	funcMap["/add"] = man.handelAdd
	funcMap["/test"] = man.handelTest
	funcMap["/login"] = man.handelLogin
	funcMap["/logout"] = man.handelLogout
	funcMap["/status"] = man.handelStatus
	funcMap["/query"] = man.handelQuery
	funcMap["/fetch"] = man.handelFetch

	if fn, has := funcMap[req.URL.Path]; has {
		fn(w, req, ctx)
	} else {
		http.NotFound(w, req)
	}

}

func (man *ProxyManager) handelIndex(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	values := ctx.values
	count := man.proxyPool.Count.Clone()
	values["proxy_count_suc"] = count.Success
	values["proxy_count_failed"] = count.Failed
	values["proxy_count"] = man.proxyPool.GetProxyNumbers()
	values["proxies"] = man.proxyPool.ActiveList()

	code := renderHTML("index.html", values, true)
	w.Write([]byte(code))
}

func (man *ProxyManager) handelAdd(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	values := ctx.values
	doPost := func() {
		if !ctx.isAdmin() {
			ctx.addLogMsg("not admin")
			w.Write([]byte("<script>alert('must admin');</script>"))
			return
		}
		proxy := req.PostFormValue("proxy")
		isApi := proxy != ""
		var proxysTxt string
		if isApi {
			proxysTxt = "proxy=" + proxy
		} else {
			proxysTxt = req.PostFormValue("proxies")
		}

		txtFile := str_util.NewTxtFileFromString(proxysTxt)
		proxies, _ := man.proxyPool.loadProxiesFromTxtFile(txtFile)
		if proxies.Total() == 0 {
			ctx.addLogMsg("no proxy, form input:[", proxysTxt, "]")
			w.Write([]byte("<script>alert('no proxy');</script>"))
			return
		}
		n := 0
		proxies.Range(func(proxyURL string, proxy *Proxy) bool {
			if man.proxyPool.addProxy(proxy) {
				n++
			}
			return true
		})

		if n > 0 {
			go man.proxyPool.runTest()
		}
		w.Write([]byte(fmt.Sprintf("<script>alert('add %d new proxy');</script>", n)))
		ctx.addLogMsg("add new proxy total:", n)
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

func (man *ProxyManager) handelAbout(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	values := ctx.values
	code := renderHTML("about.html", values, true)
	w.Write([]byte(code))
}

func (man *ProxyManager) handelLogout(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	cookie := &http.Cookie{Name: cookieName, Value: "", Path: "/"}
	http.SetCookie(w, cookie)
	if _, _, ok := req.BasicAuth(); ok {
		w.WriteHeader(401)
		w.Write([]byte("<script>location.reload()</script>"))
		return
	}
	http.Redirect(w, req, "/", 302)
}

func (man *ProxyManager) handelStatus(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	values := make(map[string]interface{})
	values["StartTime"] = man.startTime.Format(timeFormatStd)
	values["Version"] = version
	values["Request"] = man.proxyPool.Count
	values["AliveCheckURL"] = man.config.AliveCheckURL
	values["AliveCheckInterval"] = man.config.getCheckInterval().String()
	values["Timeout"] = man.proxyPool.config.getTimeout().String()
	values["ProxyDetail"] = man.proxyPool.GetProxyNumbers()
	bs, _ := json.Marshal(values)
	w.Write(bs)
}

// handelTest  测试一个代理是否可以正常使用
func (man *ProxyManager) handelTest(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	values := ctx.values
	doPost := func() {
		token, err := strconv.ParseInt(req.PostFormValue("token"), 10, 64)
		if err != nil {
			ctx.addLogMsg("invalid token", err)
			w.Write([]byte("invalid token"))
			return
		}
		urlStr := strings.TrimSpace(req.PostFormValue(fmt.Sprintf("url_%d", token-man.startTime.UnixNano())))
		obj, err := url.Parse(urlStr)
		if err != nil || !strings.HasPrefix(obj.Scheme, "http") {
			ctx.addLogMsg("test with invalid url:", urlStr)
			w.Write([]byte(fmt.Sprintf("invalid url [%s], err:%v", urlStr, err)))
			return
		}
		proxyStr := strings.TrimSpace(req.PostFormValue("proxy"))
		ctx.addLogMsg("test proxy [", proxyStr, "], url [", urlStr, "]")

		if proxyStr != "" {
			_, err := url.Parse(proxyStr)
			if err != nil {
				ctx.addLogMsg("proxy info err:", err)
				w.Write([]byte(fmt.Sprintf("wrong proxy info [%s],err:%v", proxyStr, err)))
				return
			}
			proxy := newProxy(proxyStr)
			resp, err := doRequestGet(urlStr, proxy, man.config.getTimeout())
			if err != nil {
				w.WriteHeader(502)
				w.Write([]byte(fmt.Sprintf("can not get [%s] via [%s]\nerr:%s", urlStr, proxyStr, err)))
				ctx.addLogMsg("failed, url=", urlStr, ",err=", err)
				return
			}
			copyHeaders(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
			resp.Body.Close()

		} else {
			reqTest, _ := http.NewRequest("GET", urlStr, nil)
			reqTest.SetBasicAuth(defaultTestUser.Name, defaultTestUser.Password)
			reqTest.Header.Set(proxyAuthorizationHeader, reqTest.Header.Get("Authorization"))
			reqTest.Header.Del("Authorization")

			man.httpClient.ServeHTTP(w, reqTest)
		}

	}

	switch req.Method {
	case "GET":
		nowInt := time.Now().UnixNano()
		values["url_name"] = fmt.Sprintf("url_%d", nowInt)
		values["checkURL"] = man.config.getAliveCheckURL()
		values["token"] = strconv.FormatInt(man.startTime.UnixNano()+nowInt, 10)

		code := renderHTML("test.html", values, true)
		w.Write([]byte(code))
		return
	case "POST":
		doPost()
		return

	}
	http.NotFound(w, req)
}

func (man *ProxyManager) handelLogin(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	values := ctx.values
	if req.Method == "POST" {
		name := req.PostFormValue("name")
		psw := req.PostFormValue("psw")
		if user, has := man.users[name]; has && user.pswEq(psw) {
			ctx.addLogMsg("login suc,name=", name)
			cookie := &http.Cookie{
				Name:    cookieName,
				Value:   fmt.Sprintf("%s:%s", name, user.PswEnc()),
				Path:    "/",
				Expires: time.Now().Add(86400 * time.Second),
			}
			http.SetCookie(w, cookie)
			w.Write([]byte("<script>parent.location.href='/'</script>"))
		} else {
			ctx.addLogMsg("login failed,name=", name, "psw=", psw)
			w.Write([]byte("<script>alert('login failed')</script>"))
		}
	} else {
		code := renderHTML("login.html", values, true)
		w.Write([]byte(code))
	}
}

func (man *ProxyManager) handelCheckLogin(req *http.Request, ctx *webRequestCtx) (user *User, isLogin bool) {
	if req == nil {
		return nil, false
	}
	if u, p, ok := req.BasicAuth(); ok {
		user = man.getUser(u)
		if user != nil && user.Password == p {
			return user, true
		}
		return nil, false
	}

	if req.Method == "POST" {
		if pswMd5 := req.PostFormValue("psw_md5"); pswMd5 != "" {
			userName := req.PostFormValue("user_name")
			user = man.getUser(userName)
			if user != nil && user.PasswordMd5 == pswMd5 {
				return user, true
			}
			ctx.addLogMsg("check login with psw_md5 failed, user_name=[", userName, "],psw_md5=[", pswMd5, "]")
			return nil, false
		}
	}
	cookie, err := req.Cookie(cookieName)
	if err != nil {
		return nil, false
	}
	info := strings.SplitN(cookie.Value, ":", 2)
	if len(info) != 2 {
		return nil, false
	}
	user = man.getUser(info[0])
	if user != nil && user.PswEnc() == info[1] {
		return user, true
	}
	return nil, false
}

func renderHTML(fileName string, values map[string]interface{}, layout bool) string {
	html := AssetGetContent("tpl/" + fileName)
	myFn := template.FuncMap{
		"myTime": func(t time.Time) string {
			return t.Format("20060102 15:04:05")
		},
		"myDur": func(d time.Duration) string {
			return fmt.Sprintf("%.03f s", d.Seconds())
		},
	}
	tpl, _ := template.New("page").Funcs(myFn).Parse(string(html))

	var bf []byte
	w := bytes.NewBuffer(bf)
	tpl.Execute(w, values)
	body := w.String()
	if layout {
		values["body"] = body
		return renderHTML("layout.html", values, false)
	}
	return body
}

func (man *ProxyManager) handelQuery(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	if !ctx.isLogin {
		notLoginHandler(w, req)
		return
	}
	qs := req.URL.Query()
	queryURL := qs.Get("url")
	if len(queryURL) == 0 {
		w.WriteHeader(400)
		w.Write([]byte("url param is required"))
		return
	}
	method := qs.Get("method")
	if len(method) == 0 {
		method = http.MethodGet
	}
	request, err := http.NewRequest(method, queryURL, req.Body)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte("build request failed: " + err.Error()))
		return
	}

	headers := qs.Get("headers")
	if len(headers) > 0 {
		var hs http.Header
		if err = json.Unmarshal([]byte(headers), &hs); err != nil {
			w.WriteHeader(400)
			w.Write([]byte("parser headers failed: " + err.Error()))
			return
		}
		request.Header = hs
	}

	request.SetBasicAuth(defaultTestUser.Name, defaultTestUser.Password)
	request.Header.Set(proxyAuthorizationHeader, request.Header.Get("Authorization"))
	request.Header.Del("Authorization")

	man.httpClient.ServeHTTP(w, request)
}

func (man *ProxyManager) handelFetch(w http.ResponseWriter, req *http.Request, ctx *webRequestCtx) {
	if !ctx.isLogin {
		data := map[string]interface{}{
			"ErrNo": 1,
			"Proxy": "proxy auth failed",
		}
		writeJSON(w, http.StatusBadRequest, data)
		return
	}
	proxy, err := man.proxyPool.getOneProxy(ctx.user.Name)
	if err != nil {
		data := map[string]interface{}{
			"ErrNo":   2,
			"Message": err.Error(),
		}
		writeJSON(w, http.StatusBadGateway, data)
		return
	}
	data := map[string]interface{}{
		"ErrNo": 0,
		"Proxy": proxy.proxy,
	}
	writeJSON(w, http.StatusOK, data)
}

func notLoginHandler(w http.ResponseWriter, _ *http.Request) {
	// w.Header().Set("WWW-authenticate", "Basic realm=auth need")
	// w.WriteHeader(http.StatusUnauthorized)
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("proxy auth failed"))
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	bf, _ := json.Marshal(data)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(bf)
}
