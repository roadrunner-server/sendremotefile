package sendremotefile

import (
	"sync"
)

const (
	OneMB  uint = 1024 * 1024 * 1
	FiveMB uint = 1024 * 1024 * 5
	TenMB  uint = 1024 * 1024 * 10
)

type bpool struct {
	*sync.Map
}

func NewBytePool() *bpool {
	var frameChunkedPool = &sync.Map{}
	var preallocate = &sync.Once{}
	preallocate.Do(internalAllocate(frameChunkedPool))

	return &bpool{
		frameChunkedPool,
	}
}

func internalAllocate(frameChunkedPool *sync.Map) func() {
	return func() {
		pool1 := &sync.Pool{
			New: func() any {
				data := make([]byte, OneMB)
				return &data
			},
		}
		pool5 := &sync.Pool{
			New: func() any {
				data := make([]byte, FiveMB)
				return &data
			},
		}
		pool10 := &sync.Pool{
			New: func() any {
				data := make([]byte, TenMB)
				return &data
			},
		}

		frameChunkedPool.Store(OneMB, pool1)
		frameChunkedPool.Store(FiveMB, pool5)
		frameChunkedPool.Store(TenMB, pool10)
	}
}

func (bpool *bpool) get(size uint) *[]byte {
	switch {
	case size <= OneMB:
		val, _ := bpool.Load(OneMB)
		return val.(*sync.Pool).Get().(*[]byte)
	case size <= FiveMB:
		val, _ := bpool.Load(FiveMB)
		return val.(*sync.Pool).Get().(*[]byte)
	default:
		val, _ := bpool.Load(TenMB)
		return val.(*sync.Pool).Get().(*[]byte)
	}
}

func (bpool *bpool) put(size uint, data *[]byte) {
	switch {
	case size <= OneMB:
		pool, _ := bpool.Load(OneMB)
		pool.(*sync.Pool).Put(data)
		return
	case size <= FiveMB:
		pool, _ := bpool.Load(FiveMB)
		pool.(*sync.Pool).Put(data)
		return
	default:
		pool, _ := bpool.Load(TenMB)
		pool.(*sync.Pool).Put(data)
		return
	}
}
