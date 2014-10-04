package manager

import (
	"fmt"
	"sync/atomic"
)

type ProxyCount struct {
	Total   int64
	Success int64
	Failed  int64
}

func NewProxyCount() *ProxyCount {
	return &ProxyCount{}
}

func (count *ProxyCount) MarkStatus(status PROXY_USED_STATUS) {
	if status == PROXY_USED_SUC {
		atomic.AddInt64(&count.Success, 1)
	} else if status == PROXY_USED_FAILED {
		atomic.AddInt64(&count.Failed, 1)
	}
	atomic.AddInt64(&count.Total, 1)
}

func (count *ProxyCount) String() string {
	return fmt.Sprintf("total:%d,success:%d,failed:%d", count.Total, count.Success, count.Failed)
}
