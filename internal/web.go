package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/xanygo/anygo/ds/xcmp"
	"github.com/xanygo/anygo/ds/xslice"
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

var web = &adminWeb{}

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
	values["CheckInterval"] = getCheckInterval().String()

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

	funcMap["/"] = man.handleIndex
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

var sortChain = xcmp.Chain[*proxyEntry](
	xcmp.OrderAsc(func(t *proxyEntry) string { return t.Base.Proxy }),
	xcmp.OrderAsc(func(t *proxyEntry) int64 {
		tm := t.State.LastCheck.Load()
		if tm.IsZero() {
			return math.MaxInt64
		}
		return tm.UnixNano()
	}),
)

type KV struct {
	Key   string
	Value any
}

func (man *adminWeb) handleIndex(w http.ResponseWriter, _ *http.Request, ctx *webCtx) {
	values := ctx.values
	usedTotal := defaultRelay.usedTotal.Load()
	usedSuccess := defaultRelay.usedSuccess.Load()

	values["Status"] = []KV{
		{Key: "Server Start", Value: xattr.StartTime().Format(timeFormatStd)},
		{Key: "CheckInterval", Value: getCheckInterval().String()},
		{Key: "Proxy Total", Value: pool.all.Total()},
		{Key: "Proxy Active", Value: pool.active.Total()},
		{Key: "Used Total", Value: usedTotal},
		{Key: "Used Success", Value: usedSuccess},
		{Key: "Used Fail", Value: usedTotal - usedSuccess},
	}

	active := pool.active.All()
	slices.SortFunc(active, sortChain)
	values["active"] = active
	all := pool.all.All()
	other := xslice.Filter(all, func(index int, item *proxyEntry, okTotal int) bool {
		return !item.IsOk()
	})
	slices.SortFunc(other, sortChain)
	values["other"] = other

	code := renderHTML("index.html", values, true)
	_, _ = w.Write(code)
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

		_, _ = fmt.Fprintf(w, "<script>alert('add %d new proxy');</script>", n)
		ctx.addLogMsg("add new proxy total:", n)
	}

	switch req.Method {
	case "GET":
		code := renderHTML("add.html", values, true)
		_, _ = w.Write(code)
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
	_, _ = w.Write(code)
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
		"Checker": map[string]any{
			"CheckURL": getAliveCheckURL(),
			"Interval": getCheckInterval().String(),
			"Jobs":     len(pool.checkerJobs),
		},
		"Timeout":      getProxyTimeout().String(),
		"ProxyDetail":  pool.GetProxyNumbers(),
		"NumGoroutine": runtime.NumGoroutine(),
	}

	writeJSON(w, 200, values)
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
		_, _ = w.Write(code)
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
		_, _ = w.Write(code)
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

func renderHTML(fileName string, values map[string]any, layout bool) []byte {
	w := &bytes.Buffer{}
	err := tpl.ExecuteTemplate(w, fileName, values)
	if err != nil {
		w.WriteString("reader error:" + err.Error())
	}
	if !layout {
		return w.Bytes()
	}
	values["body"] = w.String()
	return renderHTML("layout.html", values, false)
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
	limit := xurl.IntDef(req.URL.Query(), "limit", 0)
	timeout := xurl.IntDef(req.URL.Query(), "timeout", 0)
	ret := pool.DynClean(limit, timeout)
	writeJSON(w, http.StatusOK, ret)
}

func notLoginHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte("proxy auth failed"))
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	bf, _ := json.Marshal(data)
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(bf)
}
