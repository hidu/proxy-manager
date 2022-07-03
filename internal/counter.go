package internal

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

func (count *proxyCount) Clone() proxyCount {
	return proxyCount{
		Total:   atomic.LoadInt64(&count.Total),
		Success: atomic.LoadInt64(&count.Success),
		Failed:  atomic.LoadInt64(&count.Failed),
	}
}

func (count *proxyCount) String() string {
	return fmt.Sprintf("Total:%d, Success:%d, Failed:%d",
		atomic.LoadInt64(&count.Total),
		atomic.LoadInt64(&count.Success),
		atomic.LoadInt64(&count.Failed),
	)
}

type GroupNumbers map[string]int

func newGroupNumbers() GroupNumbers {
	return make(GroupNumbers)
}

func (c GroupNumbers) Add(item string, n int) {
	if _, has := c[item]; !has {
		c[item] = 0
	}
	c[item] += n
}

func (c GroupNumbers) Get(item string) int {
	if n, has := c[item]; has {
		return n
	}
	return 0
}
