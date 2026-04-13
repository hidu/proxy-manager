package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/xanygo/anygo/ds/xurl"
	"github.com/xanygo/anygo/xattr"
	"github.com/xanygo/anygo/xlog"
)

const cookieName = "x-man-proxy"

type webCtx struct {
	start   time.Time
	values  map[string]any
	user    *User
	req     *http.Request
	logMsg  string
	isLogin bool
}

func (ctx *webCtx) isAdmin() bool {
	return ctx != nil && ctx.isLogin && ctx.user.Admin
}

func (ctx *webCtx) finalLog() {
	xlog.AddAttr(ctx.req.Context(), xlog.String("CtxFields", ctx.logMsg))
}

func (ctx *webCtx) addLogMsg(msg ...any) {
	if ctx.logMsg != "" {
		ctx.logMsg += ","
	}
	ctx.logMsg += fmt.Sprint(msg...)
}

type adminWeb struct{}

func (man *adminWeb) serveAdminPage(w http.ResponseWriter, req *http.Request) {
	ctx := &webCtx{
		req:   req,
		start: time.Now(),
	}
	defer ctx.finalLog()

	if strings.HasPrefix(req.URL.Path, "/asset/") {
		http.FileServer(http.FS(files)).ServeHTTP(w, req)
		return
	}

	user, isLogin := man.handleCheckLogin(req, ctx)

	values := make(map[string]any)

	values["title"] = xattr.GetDefault[string]("SiteTitle", "")
	values["notice"] = xattr.GetDefault[string]("SiteNotice", "")
	values["AliveCheckInterval"] = getCheckInterval().String()

	values["subTitle"] = ""
	values["isLogin"] = isLogin

	if isLogin {
		values["uname"] = user.Name
		values["isAdmin"] = user.Admin
	} else {
		values["uname"] = ""
		values["isAdmin"] = false
	}

	values["startTime"] = xattr.StartTime().Format(timeFormatStd)
	values["version"] = version

	values["Listen"] = xattr.AppMain().MustGetListen("main")

	// values["proxyReqTotal"] = pool.Count.Total

	_host, _port, _ := getHostPortFromReq(req)
	values["proxy_host"] = _host
	values["proxy_port"] = _port

	ctx.values = values
	ctx.user = user
	ctx.isLogin = isLogin

	funcMap := make(map[string]func(w http.ResponseWriter, req *http.Request, ctx *webCtx))

	funcMap["/"] = man.handelIndex
	funcMap["/about"] = man.handleAbout
	funcMap["/add"] = man.handleAdd
	funcMap["/test"] = man.handleTest
	funcMap["/login"] = man.handleLogin
	funcMap["/logout"] = man.handleLogout
	funcMap["/status"] = man.handleStatus
	funcMap["/query"] = man.handleQuery
	funcMap["/fetch"] = man.handleFetch
	funcMap["/clean"] = man.handeClean

	if fn, has := funcMap[req.URL.Path]; has {
		fn(w, req, ctx)
	} else {
		http.NotFound(w, req)
	}
}

func (man *adminWeb) handelIndex(w http.ResponseWriter, _ *http.Request, ctx *webCtx) {
	values := ctx.values
	usedTotal := defaultRelay.usedTotal.Load()
	usedSuccess := defaultRelay.usedSuccess.Load()
	values["proxyCntUsedTotal"] = usedTotal
	values["proxyCntUsedSuc"] = usedSuccess
	values["proxyCntUsedFail"] = usedTotal - usedSuccess
	values["proxy_count"] = pool.GetProxyNumbers()
	values["proxies"] = pool.All()

	code := renderHTML("index.html", values, true)
	_, _ = w.Write([]byte(code))
}

// handleAdd 添加新代理地址
func (man *adminWeb) handleAdd(w http.ResponseWriter, req *http.Request, ctx *webCtx) {
	values := ctx.values
	doPost := func() {
		if !ctx.isAdmin() {
			ctx.addLogMsg("not admin")
			_, _ = w.Write([]byte("<script>alert('must admin');</script>"))
			return
		}
		str := req.PostFormValue("proxy")
		isApi := str != ""
		var proxiesTxt string
		if isApi {
			proxiesTxt = "proxy=" + str
		} else {
			proxiesTxt = req.PostFormValue("proxies")
		}

		proxies := parserProxiesFromTxt(proxiesTxt)
		if proxies.Total() == 0 {
			ctx.addLogMsg("no proxy, form input:[", proxiesTxt, "]")
			_, _ = w.Write([]byte("<script>alert('no proxy');</script>"))
			return
		}
		n := 0
		proxies.Range(func(proxyURL string, p *proxyEntry) bool {
			if pool.all.Add(p) {
				n++
				pool.dyn.Add(p)
			}
			return true
		})

		if n > 0 {
			go pool.runTest()
			pool.SaveTempToFile()
		}
		_, _ = fmt.Fprintf(w, "<script>alert('add %d new proxy');</script>", n)
		ctx.addLogMsg("add new proxy total:", n)
	}

	switch req.Method {
	case "GET":
		code := renderHTML("add.html", values, true)
		_, _ = w.Write([]byte(code))
		return
	case "POST":
		doPost()
		return
	}
	http.NotFound(w, req)
}

func (man *adminWeb) handleAbout(w http.ResponseWriter, _ *http.Request, ctx *webCtx) {
	values := ctx.values
	code := renderHTML("about.html", values, true)
	_, _ = w.Write([]byte(code))
}

func (man *adminWeb) handleLogout(w http.ResponseWriter, req *http.Request, _ *webCtx) {
	cookie := &http.Cookie{Name: cookieName, Value: "", Path: "/"}
	http.SetCookie(w, cookie)
	if _, _, ok := req.BasicAuth(); ok {
		w.WriteHeader(401)
		_, _ = w.Write([]byte("<script>location.reload()</script>"))
		return
	}
	http.Redirect(w, req, "/", http.StatusFound)
}

func (man *adminWeb) handleStatus(w http.ResponseWriter, _ *http.Request, _ *webCtx) {
	values := map[string]any{
		"StartTime": xattr.StartTime().Format(timeFormatStd),
		"Version":   version,
		"Request": map[string]any{
			"Total":   defaultRelay.usedTotal.Load(),
			"Success": defaultRelay.usedSuccess.Load(),
		},
		"AliveCheckURL":      getAliveCheckURL(),
		"AliveCheckInterval": getCheckInterval().String(),
		"Timeout":            getProxyTimeout().String(),
		"ProxyDetail":        pool.GetProxyNumbers(),
		"NumGoroutine":       runtime.NumGoroutine(),
	}

	bs, _ := json.Marshal(values)
	_, _ = w.Write(bs)
}

var staticToken string

func init() {
	staticToken = strconv.FormatInt(xattr.StartTime().UnixNano(), 10)
}

// handleTest  测试一个代理是否可以正常使用
func (man *adminWeb) handleTest(w http.ResponseWriter, req *http.Request, ctx *webCtx) {
	values := ctx.values
	doPost := func() {
		token := req.PostFormValue("token")
		if token != staticToken {
			ctx.addLogMsg("invalid token", token)
			_, _ = w.Write([]byte("invalid token"))
			return
		}
		urlStr := strings.TrimSpace(req.PostFormValue("url"))
		obj, err := url.Parse(urlStr)
		if err != nil || !strings.HasPrefix(obj.Scheme, "http") {
			ctx.addLogMsg("test with invalid url:", urlStr)
			_, _ = fmt.Fprintf(w, "invalid input url [%q], err: %v", urlStr, err)
			return
		}
		proxyStr := strings.TrimSpace(req.PostFormValue("proxy"))
		ctx.addLogMsg("test proxy [", proxyStr, "], url [", urlStr, "]")

		var testResult bool

		var pu *url.URL
		if proxyStr != "" {
			pu, err = url.Parse(proxyStr)
			if err != nil {
				ctx.addLogMsg("proxy info err:", err)
				_, _ = fmt.Fprintf(w, "wrong proxy info [%s],err:%v", proxyStr, err)
				return
			}
		} else {
			pe, err := pool.getOneProxyActive("test")
			if err != nil {
				w.Write([]byte("getOneProxyActive failed:" + err.Error()))
				return
			}
			pu = pe.Base.URL
			pe.State.UsedTotal.Add(1)
			defer func() {
				if testResult {
					pe.State.UsedSuccess.Add(1)
				}
			}()
		}
		resp, err := httpGetByProxyURL(req.Context(), urlStr, pu)
		if err != nil {
			w.WriteHeader(502)
			_, _ = fmt.Fprintf(w, "can not get [%s] via [%s]\nerr:%s", urlStr, proxyStr, err)
			ctx.addLogMsg("failed, url=", urlStr, ",err=", err)
			return
		}
		testResult = true
		defer resp.Body.Close()
		copyProxyResponseHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}

	switch req.Method {
	case "GET":
		values["checkURL"] = getAliveCheckURL()
		values["token"] = staticToken

		code := renderHTML("test.html", values, true)
		_, _ = w.Write([]byte(code))
		return
	case "POST":
		doPost()
		return
	}
	http.NotFound(w, req)
}

func (man *adminWeb) handleLogin(w http.ResponseWriter, req *http.Request, ctx *webCtx) {
	values := ctx.values
	if req.Method == "POST" {
		name := req.PostFormValue("name")
		psw := req.PostFormValue("psw")
		user := getUser(name)
		if user != nil && user.pswEq(psw) {
			ctx.addLogMsg("login suc,name=", name)
			cookie := &http.Cookie{
				Name:    cookieName,
				Value:   fmt.Sprintf("%s:%s", name, user.PswEnc()),
				Path:    "/",
				Expires: time.Now().Add(86400 * time.Second),
			}
			http.SetCookie(w, cookie)
			_, _ = w.Write([]byte("<script>parent.location.href='/'</script>"))
		} else {
			ctx.addLogMsg("login failed,name=", name, "psw=", psw)
			_, _ = w.Write([]byte("<script>alert('login failed')</script>"))
		}
	} else {
		code := renderHTML("login.html", values, true)
		_, _ = w.Write([]byte(code))
	}
}

func (man *adminWeb) handleCheckLogin(req *http.Request, ctx *webCtx) (user *User, isLogin bool) {
	if req == nil {
		return nil, false
	}
	if u, p, ok := req.BasicAuth(); ok {
		user = getUser(u)
		if user != nil && user.Password == p {
			return user, true
		}
		return nil, false
	}

	if req.Method == "POST" {
		if pswMd5 := req.PostFormValue("psw_md5"); pswMd5 != "" {
			userName := req.PostFormValue("user_name")
			user = getUser(userName)
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
	user = getUser(info[0])
	if user != nil && user.PswEnc() == info[1] {
		return user, true
	}
	return nil, false
}

func renderHTML(fileName string, values map[string]any, layout bool) string {
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
	_ = tpl.Execute(w, values)
	body := w.String()
	if layout {
		values["body"] = body
		return renderHTML("layout.html", values, false)
	}
	return body
}

func (man *adminWeb) handleQuery(w http.ResponseWriter, req *http.Request, ctx *webCtx) {
	if !ctx.isLogin {
		notLoginHandler(w, req)
		return
	}
	qs := req.URL.Query()
	queryURL := qs.Get("url")
	if len(queryURL) == 0 {
		w.WriteHeader(400)
		_, _ = w.Write([]byte("url param is required"))
		return
	}
	method := xurl.StringDef(qs, "method", http.MethodGet)
	request, err := http.NewRequestWithContext(req.Context(), strings.ToUpper(method), queryURL, req.Body)
	if err != nil {
		w.WriteHeader(400)
		_, _ = w.Write([]byte("build request failed: " + err.Error()))
		return
	}

	header := qs.Get("header")
	if len(header) > 0 {
		hs := map[string]string{}
		if err = json.Unmarshal([]byte(header), &hs); err != nil {
			w.WriteHeader(400)
			_, _ = w.Write([]byte("parser headers failed: " + err.Error()))
			return
		}
		for k, v := range hs {
			request.Header.Add(k, v)
		}
	}
	defaultRelay.forwardRequest(req.Context(), w, request, ctx.user.Name)
}

// handleFetch 获取一个代理服务器
func (man *adminWeb) handleFetch(w http.ResponseWriter, _ *http.Request, ctx *webCtx) {
	if !ctx.isLogin {
		data := map[string]any{
			"Code": 1,
			"Msg":  "proxy auth failed",
		}
		writeJSON(w, http.StatusBadRequest, data)
		ctx.addLogMsg("auth failed")
		return
	}
	one, err := pool.getOneProxyActive(ctx.user.Name)
	if err != nil {
		data := map[string]any{
			"Code": 2,
			"Msg":  err.Error(),
		}
		writeJSON(w, http.StatusBadGateway, data)
		ctx.addLogMsg("fetch failed:", err.Error())
		return
	}
	data := map[string]any{
		"Code":    0,
		"Msg":     "",
		"Proxies": []string{one.Base.Proxy},
	}
	writeJSON(w, http.StatusOK, data)
}

func (man *adminWeb) handeClean(w http.ResponseWriter, req *http.Request, ctx *webCtx) {
	if !ctx.isAdmin() {
		notLoginHandler(w, req)
		return
	}
	ret := pool.DynClean()
	writeJSON(w, http.StatusOK, ret)
}

func notLoginHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte("proxy auth failed"))
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	bf, _ := json.Marshal(data)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(bf)
}
