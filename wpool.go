package sendremotefile

import (
	"net/http"
	"sync"
)

type wpool struct {
	*sync.Pool
}

func NewWriterPool() *wpool {
	return &wpool{
		&sync.Pool{
			New: func() any {
				wr := new(writer)
				wr.code = http.StatusOK
				wr.data = make([]byte, 0, 10)
				wr.hdrToSend = make(map[string][]string, 2)
				return wr
			},
		},
	}
}

func (wp *wpool) get() *writer {
	return wp.Get().(*writer)
}

func (wp *wpool) put(w *writer) {
	w.code = http.StatusOK
	w.data = make([]byte, 0, 10)

	for k := range w.hdrToSend {
		delete(w.hdrToSend, k)
	}

	wp.Put(w)
}
