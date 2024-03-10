package sendremotefile

import (
	"errors"
	"io"
	"maps"
	"net"
	"net/http"
	"strings"
	"time"

	rrErrors "github.com/roadrunner-server/errors"
	"go.uber.org/zap"
)

const (
	rootPluginName         string        = "http"
	pluginName             string        = "sendremotefile"
	responseContentTypeKey string        = "Content-Type"
	responseContentTypeVal string        = "application/octet-stream"
	responseStatusCode     int           = http.StatusOK
	xSendRemoteHeader      string        = "X-Sendremotefile"
	defaultBufferSize      uint          = TenMB
	timeout                time.Duration = 5 * time.Second
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
	bytesPool   *bpool
	writersPool *wpool
}

func (p *Plugin) Init(cfg Configurer, log Logger) error {
	const op = rrErrors.Op("sendremotefile_plugin_init")

	if !cfg.Has(rootPluginName) {
		return rrErrors.E(op, rrErrors.Disabled)
	}

	p.log = log.NamedLogger(pluginName)
	p.bytesPool = NewBytePool()
	p.writersPool = NewWriterPool()

	return nil
}

func (p *Plugin) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rrWriter := p.writersPool.get()
		defer func() {
			p.writersPool.put(rrWriter)
			_ = r.Body.Close()
		}()

		next.ServeHTTP(rrWriter, r)

		// if there is no X-Sendremotefile header from the PHP worker, just return
		if url := rrWriter.Header().Get(xSendRemoteHeader); url == "" {
			// re-add original headers, status code and body
			maps.Copy(w.Header(), rrWriter.Header())
			w.WriteHeader(rrWriter.code)
			if len(rrWriter.data) > 0 {
				// write a body if exists
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
			p.log.Error("header value must start with http")
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
			err = resp.Body.Close()
			if err != nil {
				p.log.Error("failed to close upstream response body", zap.Error(err))
			}
		}()

		if resp.StatusCode != http.StatusOK {
			p.log.Error("invalid upstream response status code", zap.Int("rr_response_code", rrWriter.code), zap.Int("remotefile_response_code", resp.StatusCode))
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		var pl = defaultBufferSize
		if cl := resp.ContentLength; cl > 0 {
			pl = uint(cl)
		}

		pb := p.bytesPool.get(pl)

		// re-add original headers
		maps.Copy(w.Header(), rrWriter.Header())
		// overwrite content-type header
		w.Header().Set(responseContentTypeKey, responseContentTypeVal)
		w.WriteHeader(responseStatusCode)

		rc := http.NewResponseController(w)

		for {
			nr, er := resp.Body.Read(*pb)

			if nr > 0 {
				nw, ew := w.Write((*pb)[:nr])

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

		p.bytesPool.put(pl, pb)
	})
}

// Middleware/plugin name.
func (p *Plugin) Name() string {
	return pluginName
}
