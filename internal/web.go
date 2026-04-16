package internal

import (
	"bytes"
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
	"github.com/xanygo/anygo/safely"
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

	aw.router.GetFunc("/logout", aw.handleLogout)
	aw.router.GetFunc("/clean", aw.handleClean)
	aw.router.GetFunc("/cancel", aw.handleCancel)
	aw.router.GetFunc("/start_check", aw.handleStartCheck)

	// 支持多种 Method
	aw.router.HandleFunc("/fetch", aw.handleFetch)   // 通过代理访问
	aw.router.HandleFunc("/direct", aw.handleDirect) // 直接访问

	aw.router.GetFunc("/", aw.handleIndex)
}

var ctxKey = xctx.NewKey()

func (aw *adminWeb) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	wc := &webCtx{
		req:   req,
		start: time.Now(),
	}
	defer wc.finalLog()

	user, isLogin := aw.preCheckLogin(req)

	wc.user = user
	wc.isLogin = isLogin

	values := make(map[string]any)

	values["title"] = xattr.GetDefault[string]("SiteTitle", "")
	values["notice"] = xattr.GetDefault[string]("SiteNotice", "")
	values["CheckInterval"] = getCheckInterval().String()

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

	_host, _port, _ := getHostPortFromReq(req)
	values["proxy_host"] = _host
	values["proxy_port"] = _port

	wc.values = values

	reqCtx := context.WithValue(req.Context(), ctxKey, wc)
	req = req.WithContext(reqCtx)
	aw.router.ServeHTTP(w, req)
}

func (aw *adminWeb) preCheckLogin(req *http.Request) (user *User, isLogin bool) {
	if req == nil {
		return nil, false
	}
	if u, p, ok := req.BasicAuth(); ok {
		user = getUser(u)
		if user != nil && user.PasswordMd5 == p {
			return user, true
		}
		return nil, false
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
	wc := aw.getWebCtx(req.Context())
	values := wc.values
	code := renderHTML("add.html", values, true)
	_, _ = w.Write(code)
}

func (aw *adminWeb) handleAddPost(w http.ResponseWriter, req *http.Request) {
	wc := aw.getWebCtx(req.Context())
	if !wc.isAdmin() {
		wc.addLogMsg("not admin")
		_, _ = w.Write([]byte("<script>alert('must admin');</script>"))
		return
	}
	str := req.PostFormValue("proxy")
	proxies := parserProxiesFromTxt(str)
	if proxies.Total() == 0 {
		wc.addLogMsg("no proxy, form input:[", str, "]")
		_, _ = w.Write([]byte("<script>alert('no proxy');</script>"))
		return
	}
	var added []*proxyEntry
	proxies.Range(func(proxyURL string, p *proxyEntry) bool {
		if pool.all.Add(p) {
			pool.dyn.Add(p)
			added = append(added, p)
		}
		return true
	})

	if len(added) > 0 {
		go safely.Run(func() {
			for _, p := range added {
				pool.checkerJobs <- p
			}
		})
	}

	_, _ = fmt.Fprintf(w, "<script>alert('add %d new proxy');</script>", len(added))
	wc.addLogMsg("add new proxy total:", len(added))
}

func (aw *adminWeb) handleAbout(w http.ResponseWriter, req *http.Request) {
	wc := aw.getWebCtx(req.Context())
	values := wc.values
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
	wc := aw.getWebCtx(req.Context())
	values := wc.values
	values["checkURL"] = getProbeURL()
	values["token"] = staticToken

	code := renderHTML("test.html", values, true)
	_, _ = w.Write(code)
}

// handleTest  测试一个代理是否可以正常使用
func (aw *adminWeb) handleTestPost(w http.ResponseWriter, req *http.Request) {
	wc := aw.getWebCtx(req.Context())
	token := req.PostFormValue("token")
	if token != staticToken {
		wc.addLogMsg("invalid token", token)
		_, _ = w.Write([]byte("invalid token"))
		return
	}
	urlStr := strings.TrimSpace(req.PostFormValue("url"))
	obj, err := url.Parse(urlStr)
	if err != nil || (obj.Scheme != "http" && obj.Scheme != "https") {
		wc.addLogMsg("test with invalid url:", urlStr)
		_, _ = fmt.Fprintf(w, "invalid input url [%q], err: %v", urlStr, err)
		return
	}
	proxyStr := strings.TrimSpace(req.PostFormValue("proxy"))
	wc.addLogMsg("test proxy [", proxyStr, "], url [", urlStr, "]")

	var testResult bool

	var pu *url.URL
	if proxyStr != "" {
		pu, err = url.Parse(proxyStr)
		if err != nil {
			wc.addLogMsg("proxy info err:", err)
			_, _ = fmt.Fprintf(w, "wrong proxy info [%s],err:%v", proxyStr, err)
			return
		}
	} else {
		pe, err := pool.getOneProxyActive(req.Context(), "")
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
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(w, "can not get [%s] via [%s]\nerr:%s", urlStr, proxyStr, err)
		wc.addLogMsg("failed, url=", urlStr, ",err=", err)
		return
	}
	testResult = true
	defer resp.Body.Close()
	copyProxyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (aw *adminWeb) handleLoginGet(w http.ResponseWriter, req *http.Request) {
	wc := aw.getWebCtx(req.Context())
	values := wc.values
	code := renderHTML("login.html", values, true)
	_, _ = w.Write(code)
}

func (aw *adminWeb) handleLoginPost(w http.ResponseWriter, req *http.Request) {
	wc := aw.getWebCtx(req.Context())
	name := req.PostFormValue("name")
	psw := req.PostFormValue("psw")
	user := getUser(name)
	if user != nil && user.pswEq(psw) {
		wc.addLogMsg("login suc,name=", name)
		cookie := &http.Cookie{
			Name:     cookieName,
			Value:    fmt.Sprintf("%s:%s", name, user.PswEnc()),
			Path:     "/",
			Expires:  time.Now().Add(86400 * time.Second),
			HttpOnly: true,
		}
		http.SetCookie(w, cookie)
		_, _ = w.Write([]byte("<script>parent.location.href='/'</script>"))
	} else {
		wc.addLogMsg("login failed,name=", name, "psw=", psw)
		_, _ = w.Write([]byte("<script>alert('login failed')</script>"))
	}
}

func (aw *adminWeb) handleDirect(w http.ResponseWriter, req *http.Request) {
	ctx := contextWithProxyEntry(req.Context(), directEntry)
	req = req.WithContext(ctx)
	aw.handleFetch(w, req)
}

func (aw *adminWeb) handleFetch(w http.ResponseWriter, req *http.Request) {
	wc := aw.getWebCtx(req.Context())
	if authType() != AuthTypeNO && !wc.isLogin {
		notLoginHandler(w, req)
		return
	}
	qs := req.URL.Query()
	queryURL := qs.Get("url")
	if len(queryURL) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("url param is required"))
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("read body err:" + err.Error()))
		return
	}

	method := xurl.StringDef(qs, "method", req.Method)
	request, err := http.NewRequestWithContext(req.Context(), strings.ToUpper(method), queryURL, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("build request failed: " + err.Error()))
		return
	}
	// 确保即使重试，body 也能正常的转发
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	header := qs.Get("header")
	if header == "" {
		header = req.Header.Get("X-Man-Header")
	}
	if len(header) > 0 {
		hs := map[string]string{}
		if err = json.Unmarshal([]byte(header), &hs); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("parser headers failed: " + err.Error()))
			return
		}
		for k, v := range hs {
			request.Header.Add(k, v)
		}
	}
	filter := qs.Get("filter")
	if filter == "" {
		filter = req.Header.Get("X-Man-Filter")
	}
	attempt := xurl.IntDef(qs, "retry", getRetryWithRequest(req)) + 1
	param := forwardParam{
		Request:  request,
		Username: wc.userName(),
		Filter:   filter,
		Attempt:  max(attempt, 1),
		Format:   xurl.StringDef(qs, "format", req.Header.Get("X-Man-Format")),
	}

	defaultRelay.forwardRequest(req.Context(), w, param)
}

// handlePick 获取一个代理服务器
func (aw *adminWeb) handlePick(w http.ResponseWriter, req *http.Request) {
	wc := aw.getWebCtx(req.Context())
	if !wc.isLogin {
		data := map[string]any{
			"Code": 1,
			"Msg":  "proxy auth failed",
		}
		writeJSON(w, http.StatusBadRequest, data)
		wc.addLogMsg("auth failed")
		return
	}
	one, err := pool.getOneProxyActive(req.Context(), req.URL.Query().Get("filter"))
	if err != nil {
		data := map[string]any{
			"Code": 2,
			"Msg":  err.Error(),
		}
		writeJSON(w, http.StatusBadGateway, data)
		wc.addLogMsg("fetch failed:", err.Error())
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
	wc := aw.getWebCtx(req.Context())
	if !wc.isAdmin() {
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
	wc := aw.getWebCtx(req.Context())
	if !wc.isAdmin() {
		notLoginHandler(w, req)
		return
	}
	minute := xurl.IntDef(req.URL.Query(), "minute", 2)
	silentDeadline.Store(time.Now().Add(time.Duration(minute) * time.Minute))
	w.Write([]byte("Ok"))
}

func (aw *adminWeb) handleStartCheck(w http.ResponseWriter, req *http.Request) {
	wc := aw.getWebCtx(req.Context())
	if !wc.isAdmin() {
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
