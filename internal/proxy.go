package internal

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/xanygo/anygo/ds/xmap"
	"github.com/xanygo/anygo/ds/xslice"
	"github.com/xanygo/anygo/ds/xsync"
	"github.com/xanygo/anygo/xcfg"
	"github.com/xanygo/anygo/xio/xfs"
	"github.com/xanygo/anygo/xlog"
	"gopkg.in/yaml.v3"

	"github.com/hidu/proxy-manager/internal/transport"
)

// directEntry 使用这个，总是会从本机自己发出请求（host 和 port 不会实际使用，合法即可）
var directEntry = newProxy("direct://localhost:1")

func init() {
	if directEntry == nil {
		panic("directEntry is nil")
	}
}

func loadProxies(filename string) ([]*proxyBase, error) {
	config := &ProxiesFile{}
	err := xcfg.Parse(filename, config)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return config.Proxies, nil
}

type ProxiesFile struct {
	Proxies []*proxyBase `yaml:"Proxies"`
}

type proxyEntry struct {
	Base  *proxyBase
	State *proxyState
}

// Proxy 一个代理
type proxyBase struct {
	Proxy   string    `yaml:"Proxy"` // 代理地址，如 http://example.com:8128
	URL     *url.URL  `yaml:"-" json:"-"`
	Weight  int       `yaml:"Weight,omitempty"`
	Created time.Time `yaml:"Created,omitempty"`
	Tags    []string  `yaml:"Tags,omitempty"` // 标签，可用于筛选
}

func (b *proxyBase) ToProxy() *proxyEntry {
	var err error
	b.URL, err = url.Parse(b.Proxy)
	if err != nil {
		log.Println("proxy info wrong", err)
		return nil
	}
	return &proxyEntry{
		Base:  b,
		State: &proxyState{},
	}
}

type proxyState struct {
	LastCheck       xsync.TimeStamp     // 最后检查时间
	LastCheckOk     xsync.TimeStamp     // 最后检查正常的时间
	LastCheckStatus atomic.Int64        // 最后一次检查返回的状态码，值为200 才是正常的
	LastCheckUsed   xsync.TimeDuration  // 最后检查耗时
	CheckTimes      atomic.Int64        // 检查次数
	LastCheckMsg    xsync.Value[string] // 最后检查的消息。

	UsedTotal   atomic.Int64 // 被使用的次数
	UsedSuccess atomic.Int64 // 使用正常的次数
}

func (ps *proxyState) UsedFailed() int64 {
	return ps.UsedTotal.Load() - ps.UsedSuccess.Load()
}

func (ps *proxyState) IsStatusOk() bool {
	v := ps.LastCheckStatus.Load()
	return v == http.StatusOK || v == http.StatusNoContent
}

func newProxy(proxyURL string) *proxyEntry {
	base := &proxyBase{Proxy: proxyURL}
	var err error
	base.URL, err = url.Parse(proxyURL)
	if err != nil {
		xlog.Warn(context.Background(), "invalid proxy url", xlog.String("Proxy", proxyURL), xlog.ErrorAttr("Error", err))
		return nil
	}

	if !transport.HasScheme(base.URL.Scheme) {
		xlog.Warn(context.Background(), "invalid proxy scheme", xlog.String("Proxy", proxyURL))
		return nil
	}

	return &proxyEntry{
		Base:  base,
		State: &proxyState{},
	}
}

func (p *proxyEntry) String() string {
	return ""
}

// IsOk 是否可用状态
func (p *proxyEntry) IsOk() bool {
	return p.State.IsStatusOk()
}

func (p *proxyEntry) GetUsedTotal() int64 {
	return p.State.UsedTotal.Load()
}

var zd net.Dialer

func (p *proxyEntry) TestByDial(ctx context.Context, timeoutSeconds int) error {
	host, port, err := getHostPortFromURL(p.Base.Proxy)
	if err != nil {
		return err
	}
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = getProxyTimeout()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := zd.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

func newProxyList(items []*proxyBase) *ProxyList {
	pl := &ProxyList{
		all:  &xslice.Sync[*proxyEntry]{},
		list: &xmap.Sync[string, *proxyEntry]{},
	}
	for _, item := range items {
		if p := item.ToProxy(); p != nil {
			pl.Add(p)
		}
	}
	return pl
}

type ProxyList struct {
	all     *xslice.Sync[*proxyEntry]
	list    *xmap.Sync[string, *proxyEntry]
	nextID  atomic.Int64
	changed xsync.TimeStamp // 保存首次修改后的时间
}

func (pl *ProxyList) Range(fn func(proxyURL string, proxy *proxyEntry) bool) {
	pl.list.Range(fn)
}

func (pl *ProxyList) Add(p *proxyEntry) bool {
	_, loaded := pl.list.LoadOrStore(p.Base.Proxy, p)
	if !loaded {
		pl.updateAll()
		pl.saveChanged()
	}
	return !loaded
}

func (pl *ProxyList) saveChanged() {
	pl.changed.CompareAndSwap(time.Time{}, time.Now())
}

func (pl *ProxyList) ResetChanged() {
	pl.changed.Store(time.Time{})
}

func (pl *ProxyList) updateAll() {
	var all []*proxyEntry
	pl.Range(func(proxyURL string, proxy *proxyEntry) bool {
		all = append(all, proxy)
		return true
	})
	pl.all.Store(all)
}

func (pl *ProxyList) Remove(one *proxyEntry) bool {
	return pl.RemoveByKey(one.Base.Proxy)
}

func (pl *ProxyList) RemoveByKey(key string) bool {
	_, loaded := pl.list.LoadAndDelete(key)
	if loaded {
		pl.updateAll()
		pl.saveChanged()
	}
	return loaded
}

func (pl *ProxyList) Get(key string) *proxyEntry {
	val, _ := pl.list.Load(key)
	return val
}

func (pl *ProxyList) Total() int {
	return pl.list.Len()
}

func (pl *ProxyList) MergeTo(to *ProxyList) {
	if pl == nil {
		return
	}
	pl.list.Range(func(key string, value *proxyEntry) bool {
		to.Add(value)
		return true
	})
}

var errorNoProxy = errors.New("no active proxy")

// FilterOne 筛选过滤出一个满足条件的,
//
//	filter 格式：tag1 & tag2,tag3,[ANY]   -> 三个条件，同时有 tag1 和 tag2 或者 有tag 3，或者 任意返回一个
//	按照顺序依次返回
func (pl *ProxyList) FilterOne(filter string) (*proxyEntry, error) {
	allProxy := pl.all.Load()
	if len(allProxy) == 0 {
		return nil, errorNoProxy
	}
	fn, err := xslice.BuildTagFilter(filter, func(t *proxyEntry) []string {
		return t.Base.Tags
	}, 0)
	if err != nil {
		return nil, err
	}
	result := fn(allProxy)
	if len(result) == 0 {
		return nil, errorNoProxy
	}
	n := rand.IntN(len(result))
	return result[n], nil
}

func (pl *ProxyList) Next() *proxyEntry {
	allProxy := pl.all.Load()
	if len(allProxy) == 0 {
		return nil
	}
	nextID := pl.nextID.Add(1)
	index := int(nextID) % len(allProxy)
	return allProxy[index]
}

func (pl *ProxyList) String() string {
	file := &ProxiesFile{}
	pl.Range(func(proxyURL string, proxy *proxyEntry) bool {
		file.Proxies = append(file.Proxies, proxy.Base)
		return true
	})
	bf, _ := yaml.Marshal(file)
	return string(bf)
}

func (pl *ProxyList) SaveFile(filename string) error {
	xfs.KeepDirExists(filepath.Dir(filename))
	content := pl.String()
	return os.WriteFile(filename, []byte(content), 0666)
}

func (pl *ProxyList) All() []*proxyEntry {
	return pl.all.Load()
}
