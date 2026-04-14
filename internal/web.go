package internal

import (
	"context"
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
	"github.com/xanygo/anygo/ds/xctx"
	"github.com/xanygo/anygo/ds/xslice"
	"github.com/xanygo/anygo/ds/xsync"
	"github.com/xanygo/anygo/ds/xurl"
	"github.com/xanygo/anygo/xattr"
	"github.com/xanygo/anygo/xhttp"
	"github.com/xanygo/anygo/xio/xfs"
	"github.com/xanygo/anygo/xlog"
	"github.com/xanygo/webr"
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

func (ctx *webCtx) userName() string {
	if ctx.user != nil {
		return ctx.user.Name
	}
	return ""
}

func (ctx *webCtx) isAdmin() bool {
	return ctx != nil && ctx.isLogin && ctx.user != nil && ctx.user.Admin
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

var web = newAdminWeb()

func newAdminWeb() *adminWeb {
	aw := &adminWeb{
		router: xhttp.NewRouter(),
	}
	aw.init()
	return aw
}

type adminWeb struct {
	router *xhttp.Router
}

func (aw *adminWeb) init() {
	assetHandler := xhttp.FSHandlers{
		&xhttp.FS{
			FS: xfs.OverlayFS{
				webr.Bootstrap(),
			},
		},
		&xhttp.FS{
			FS:      files,
			RootDir: "asset/css",
		},
	}
	aw.router.Get("/asset/*", http.StripPrefix("/asset/", assetHandler))

	aw.router.GetFunc("/about", aw.handleAbout)
	aw.router.GetFunc("/status", aw.handleStatus)
	aw.router.GetFunc("/pick", aw.handlePick)

	aw.router.GetFunc("/add", aw.handleAddGet)
	aw.router.PostFunc("/add", aw.handleAddPost)

	aw.router.GetFunc("/test", aw.handleTestGet)
	aw.router.PostFunc("/test", aw.handleTestPost)

	aw.router.GetFunc("/login", aw.handleLoginGet)
	aw.router.PostFunc("/login", aw.handleLoginPost)

	aw.router.HandleFunc("/logout", aw.handleLogout)
	aw.router.HandleFunc("/clean", aw.handleClean)
	aw.router.HandleFunc("/cancel", aw.handleCancel)
	aw.router.HandleFunc("/start_check", aw.handleStartCheck)
	aw.router.HandleFunc("/fetch", aw.handleFetch)

	aw.router.GetFunc("/", aw.handleIndex)
}

var ctxKey = xctx.NewKey()

func (aw *adminWeb) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := &webCtx{
		req:   req,
		start: time.Now(),
	}
	defer ctx.finalLog()

	// if strings.HasPrefix(req.URL.Path, "/asset/") {
	//	http.FileServer(http.FS(files)).ServeHTTP(w, req)
	//	return
	// }

	user, isLogin := aw.handleCheckLogin(req, ctx)

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

	reqCtx := context.WithValue(req.Context(), ctxKey, ctx)
	req = req.WithContext(reqCtx)
	aw.router.ServeHTTP(w, req)
}

var sortChain = xcmp.Chain[*proxyEntry](
	xcmp.OrderAsc(func(t *proxyEntry) int64 {
		tm := t.State.LastCheck.Load()
		if tm.IsZero() {
			return math.MaxInt64
		}
		return tm.UnixNano()
	}),
	xcmp.OrderAsc(func(t *proxyEntry) string { return t.Base.Proxy }),
)

type KV struct {
	Key   string
	Value any
}

func (aw *adminWeb) getWebCtx(ctx context.Context) *webCtx {
	return ctx.Value(ctxKey).(*webCtx)
}

func (aw *adminWeb) handleIndex(w http.ResponseWriter, req *http.Request) {
	values := aw.getWebCtx(req.Context()).values
	usedTotal := defaultRelay.usedTotal.Load()
	usedSuccess := defaultRelay.usedSuccess.Load()

	status := []KV{
		{Key: "Server Start", Value: xattr.StartTime().Format(timeFormatStd)},
		{Key: "Check Interval", Value: getCheckInterval().String()},

		{Key: "Proxies Total", Value: pool.all.Total()},
		{Key: "Proxies Active", Value: pool.active.Total()},

		{Key: "Usage Total", Value: usedTotal},
		{Key: "Usage Success", Value: usedSuccess},
		{Key: "Usage Fail", Value: usedTotal - usedSuccess},

		{Key: "Checker Probe URL", Value: getProbeURL()},
		{Key: "Checker Producer Running", Value: producerRunning.Load()},
		{Key: "Checker Last Check", Value: lastChecked.Load()},
	}

	if ct := silentDeadline.Load(); ct.After(time.Now()) {
		status = append(status, KV{Key: "Checker Silent Deadline", Value: ct.String()})
	}

	values["Status"] = status

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
func (aw *adminWeb) handleAddGet(w http.ResponseWriter, req *http.Request) {
	ctx := aw.getWebCtx(req.Context())
	values := ctx.values
	code := renderHTML("add.html", values, true)
	_, _ = w.Write(code)
}

func (aw *adminWeb) handleAddPost(w http.ResponseWriter, req *http.Request) {
	ctx := aw.getWebCtx(req.Context())
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

func (aw *adminWeb) handleAbout(w http.ResponseWriter, req *http.Request) {
	ctx := aw.getWebCtx(req.Context())
	values := ctx.values
	code := renderHTML("about.html", values, true)
	_, _ = w.Write(code)
}

func (aw *adminWeb) handleLogout(w http.ResponseWriter, req *http.Request) {
	cookie := &http.Cookie{Name: cookieName, Value: "", Path: "/"}
	http.SetCookie(w, cookie)
	if _, _, ok := req.BasicAuth(); ok {
		w.WriteHeader(401)
		_, _ = w.Write([]byte("<script>location.reload()</script>"))
		return
	}
	http.Redirect(w, req, "/", http.StatusFound)
}

func (aw *adminWeb) handleStatus(w http.ResponseWriter, _ *http.Request) {
	usedTotal := defaultRelay.usedTotal.Load()
	usedSuccess := defaultRelay.usedSuccess.Load()
	values := map[string]any{
		"StartTime": xattr.StartTime().Format(timeFormatStd),
		"Version":   version,
		"Checker": map[string]any{
			"ProbeURL":     getProbeURL(),
			"Interval":     getCheckInterval().String(),
			"BufferedJobs": len(pool.checkerJobs),
		},
		"Counter": map[string]any{
			"ActiveProxies": pool.active.Total(),
			"TotalProxies":  pool.all.Total(),
			"DynProxies":    pool.dyn.Total(),

			"UsageTotal":   usedTotal,
			"UsageSuccess": usedSuccess,
			"UsageFail":    usedTotal - usedSuccess,
		},
		"Timeout":      getProxyTimeout().String(),
		"NumGoroutine": runtime.NumGoroutine(),
	}

	writeJSON(w, 200, values)
}

var staticToken string

func init() {
	staticToken = strconv.FormatInt(xattr.StartTime().UnixNano(), 10)
}

// handleTest  测试一个代理是否可以正常使用
func (aw *adminWeb) handleTestGet(w http.ResponseWriter, req *http.Request) {
	ctx := aw.getWebCtx(req.Context())
	values := ctx.values
	values["checkURL"] = getProbeURL()
	values["token"] = staticToken

	code := renderHTML("test.html", values, true)
	_, _ = w.Write(code)
}

// handleTest  测试一个代理是否可以正常使用
func (aw *adminWeb) handleTestPost(w http.ResponseWriter, req *http.Request) {
	ctx := aw.getWebCtx(req.Context())
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

func (aw *adminWeb) handleLoginGet(w http.ResponseWriter, req *http.Request) {
	ctx := aw.getWebCtx(req.Context())
	values := ctx.values
	code := renderHTML("login.html", values, true)
	_, _ = w.Write(code)
}

func (aw *adminWeb) handleLoginPost(w http.ResponseWriter, req *http.Request) {
	ctx := aw.getWebCtx(req.Context())
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
}

func (aw *adminWeb) handleCheckLogin(req *http.Request, ctx *webCtx) (user *User, isLogin bool) {
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

func (aw *adminWeb) handleFetch(w http.ResponseWriter, req *http.Request) {
	ctx := aw.getWebCtx(req.Context())
	if authType() != AuthTypeNO && !ctx.isLogin {
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
	defaultRelay.forwardRequest(req.Context(), w, request, ctx.userName())
}

// handlePick 获取一个代理服务器
func (aw *adminWeb) handlePick(w http.ResponseWriter, req *http.Request) {
	ctx := aw.getWebCtx(req.Context())
	if !ctx.isLogin {
		data := map[string]any{
			"Code": 1,
			"Msg":  "proxy auth failed",
		}
		writeJSON(w, http.StatusBadRequest, data)
		ctx.addLogMsg("auth failed")
		return
	}
	one, err := pool.getOneProxyActive(ctx.userName())
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

func (aw *adminWeb) handleClean(w http.ResponseWriter, req *http.Request) {
	ctx := aw.getWebCtx(req.Context())
	if !ctx.isAdmin() {
		notLoginHandler(w, req)
		return
	}
	limit := xurl.IntDef(req.URL.Query(), "limit", 0)
	timeout := xurl.IntDef(req.URL.Query(), "timeout", 0)
	ret := pool.DynClean(limit, timeout)
	writeJSON(w, http.StatusOK, ret)
}

var silentDeadline xsync.TimeStamp

func (aw *adminWeb) handleCancel(w http.ResponseWriter, req *http.Request) {
	ctx := aw.getWebCtx(req.Context())
	if !ctx.isAdmin() {
		notLoginHandler(w, req)
		return
	}
	minute := xurl.IntDef(req.URL.Query(), "minute", 2)
	silentDeadline.Store(time.Now().Add(time.Duration(minute) * time.Minute))
	w.Write([]byte("Ok"))
}

func (aw *adminWeb) handleStartCheck(w http.ResponseWriter, req *http.Request) {
	ctx := aw.getWebCtx(req.Context())
	if !ctx.isAdmin() {
		notLoginHandler(w, req)
		return
	}
	go pool.startCheckProducer()
	w.Write([]byte("Ok"))
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
