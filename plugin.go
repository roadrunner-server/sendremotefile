package sendremotefile

import (
	"errors"
	"io"
	"maps"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	rrErrors "github.com/roadrunner-server/errors"
	"go.uber.org/zap"
)

const (
	RootPluginName    string        = "http"
	PluginName        string        = "sendremotefile"
	ContentTypeKey    string        = "Content-Type"
	ContentTypeVal    string        = "application/octet-stream"
	xSendRemoteHeader string        = "X-Sendremotefile"
	defaultBufferSize int           = 10 * 1024 * 1024 // 10MB chunks
	timeout           time.Duration = 5 * time.Second
)

type Configurer interface {
	UnmarshalKey(name string, out any) error
	Has(name string) bool
}

type Logger interface {
	NamedLogger(name string) *zap.Logger
}

type Plugin struct {
	log         *zap.Logger
	writersPool sync.Pool
}

func (p *Plugin) Init(cfg Configurer, log Logger) error {
	const op = rrErrors.Op("sendremotefile_plugin_init")

	if !cfg.Has(RootPluginName) {
		return rrErrors.E(op, rrErrors.Disabled)
	}

	p.log = log.NamedLogger(PluginName)

	p.writersPool = sync.Pool{
		New: func() any {
			wr := new(writer)
			wr.code = http.StatusOK
			wr.data = make([]byte, 0, 10)
			wr.hdrToSend = make(map[string][]string, 2)
			return wr
		},
	}

	return nil
}

func (p *Plugin) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rrWriter := p.getWriter()
		defer func() {
			p.putWriter(rrWriter)
			_ = r.Body.Close()
		}()

		next.ServeHTTP(rrWriter, r)

		// if there is no X-Sendremotefile header from the PHP worker, just return
		if url := rrWriter.Header().Get(xSendRemoteHeader); url == "" {
			// re-add original headers, status code and body
			maps.Copy(w.Header(), rrWriter.hdrToSend)
			w.WriteHeader(rrWriter.code)
			if len(rrWriter.data) > 0 {
				_, err := w.Write(rrWriter.data)
				if err != nil {
					p.log.Error("failed to write data to the response", zap.Error(err))
				}
			}

			return
		}

		// we already checked that that header exists
		url := rrWriter.Header().Get(xSendRemoteHeader)
		// delete the original X-Sendremotefile header
		rrWriter.Header().Del(xSendRemoteHeader)

		if !strings.HasPrefix(url, "http") {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		resp, err := NewClient(url, timeout).Request()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				http.Error(w, http.StatusText(http.StatusRequestTimeout), http.StatusRequestTimeout)
				return
			}

			p.log.Error("failed to request from the upstream", zap.Error(err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		defer func() {
			_ = resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK {
			p.log.Error("invalid upstream response status code", zap.Int("status_code", resp.StatusCode))
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		var buf []byte
		// do not allocate large buffer for the small files
		if size := resp.ContentLength; size > 0 && size < int64(defaultBufferSize) {
			// allocate based on provided content length
			buf = make([]byte, size)
		} else {
			// allocate default buffer
			buf = make([]byte, defaultBufferSize)
		}

		// re-add original headers
		maps.Copy(w.Header(), rrWriter.hdrToSend)
		// overwrite content-type header
		w.Header().Set(ContentTypeKey, ContentTypeVal)

		rc := http.NewResponseController(w)

		for {
			nr, er := resp.Body.Read(buf)

			if nr > 0 {
				nw, ew := w.Write(buf[0:nr])

				if nw > 0 {
					if ef := rc.Flush(); ef != nil {
						p.log.Error("failed to flush data to the downstream response", zap.Error(ef))
						break
					}
				}

				if ew != nil {
					p.log.Error("failed to write data to the downstream response", zap.Error(ew))
					break
				}
			}

			if er == io.EOF {
				break
			}

			if er != nil {
				p.log.Error("failed to read data from the upstream response", zap.Error(er))
				break
			}
		}
	})
}

// Middleware/plugin name.
func (p *Plugin) Name() string {
	return PluginName
}

func (p *Plugin) getWriter() *writer {
	return p.writersPool.Get().(*writer)
}

func (p *Plugin) putWriter(w *writer) {
	w.code = http.StatusOK
	w.data = make([]byte, 0, 10)

	for k := range w.hdrToSend {
		delete(w.hdrToSend, k)
	}

	p.writersPool.Put(w)
}
