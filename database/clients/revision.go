package clients

import "sync/atomic"

var revision atomic.Uint64

func Revision() uint64 {
	return revision.Load()
}

func NotifyChanged() {
	revision.Add(1)
}
