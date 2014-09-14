package manager

import (
	"fmt"
	"sync/atomic"
)

type ProxyCount struct {
	total   int64
	success int64
	failed  int64
}

func NewProxyCount() *ProxyCount {
	return &ProxyCount{}
}

func (count *ProxyCount) MarkStatus(status PROXY_USED_STATUS) {
	if status == PROXY_USED_SUC {
		atomic.AddInt64(&count.success, 1)
	} else if status == PROXY_USED_FAILED {
		atomic.AddInt64(&count.failed, 1)
	}
	atomic.AddInt64(&count.total, 1)
}

func (count *ProxyCount) String() string {
	return fmt.Sprintf("total:%d,success:%d,failed:%d", count.total, count.success, count.failed)
}
