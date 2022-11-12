package counter

import "sync/atomic"

type Counter int64

func (c *Counter) Inc() int64 {
	return atomic.AddInt64((*int64)(c), 1)
}

func (c *Counter) Get() int64 {
	return atomic.LoadInt64((*int64)(c))
}
