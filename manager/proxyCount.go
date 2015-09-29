package manager

import (
	"fmt"
	"sync/atomic"
)

type proxyCount struct {
	Total   int64
	Success int64
	Failed  int64
}

func newProxyCount() *proxyCount {
	return &proxyCount{}
}

func (count *proxyCount) MarkStatus(status proxyUsedStatus) {
	if status == proxyUsedSuc {
		atomic.AddInt64(&count.Success, 1)
	} else if status == proxyUsedFailed {
		atomic.AddInt64(&count.Failed, 1)
	}
	atomic.AddInt64(&count.Total, 1)
}

func (count *proxyCount) String() string {
	return fmt.Sprintf("total:%d,success:%d,failed:%d", count.Total, count.Success, count.Failed)
}
