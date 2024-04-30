package sendremotefile

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	mocklogger "tests/mock"

	toxiproxy "github.com/Shopify/toxiproxy/client"
	"github.com/roadrunner-server/config/v4"
	"github.com/roadrunner-server/endure/v2"
	httpPlugin "github.com/roadrunner-server/http/v4"
	"github.com/roadrunner-server/sendremotefile/v4"
	"github.com/roadrunner-server/server/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var proxy *toxiproxy.Proxy

func init() {
	client := toxiproxy.NewClient("127.0.0.1:8474")
	var err error
	proxy, err = client.CreateProxy("TestStorageDown", "localhost:26379", "localhost:9000")
	if err != nil {
		panic(err)
	}
}

func TestStorageDown(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-with-sendremotefile.yaml",
		Prefix:  "rr",
	}

	l, oLogger := mocklogger.ZapTestLogger(zap.DebugLevel)

	err := cont.RegisterAll(
		cfg,
		l,
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&sendremotefile.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	require.NoError(t, err)

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)

	proxy.Disable()

	r, err := http.DefaultClient.Get("http://127.0.0.1:18953/minio-file")
	require.NoError(t, err)

	assert.Equal(t, 500, r.StatusCode)
	assert.Equal(t, 1, oLogger.FilterMessageSnippet("failed to request from the upstream").Len())

	b, err := io.ReadAll(r.Body)
	require.NoError(t, err)

	assert.Equal(t, "Internal Server Error\n", string(b))

	err = r.Body.Close()
	require.NoError(t, err)

	stopCh <- struct{}{}
	wg.Wait()
}

func TestStorageSlowConnectionDown(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-with-sendremotefile.yaml",
		Prefix:  "rr",
	}

	l, oLogger := mocklogger.ZapTestLogger(zap.DebugLevel)

	err := cont.RegisterAll(
		cfg,
		l,
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&sendremotefile.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	require.NoError(t, err)

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)

	proxy.Enable()
	proxy.AddToxic("slow_bandwidth", "bandwidth", "downstream", 1.0, toxiproxy.Attributes{
		"rate": 10,
	})

	go func() {
		time.Sleep(time.Second * 2)
		proxy.Disable()
	}()

	time.Sleep(time.Second * 1)

	r, err := http.DefaultClient.Get("http://127.0.0.1:18953/minio-file")
	require.NoError(t, err)

	assert.Equal(t, 200, r.StatusCode)

	b, err := io.ReadAll(r.Body)
	require.NoError(t, err)

	assert.Less(t, len(b), 100000)

	assert.Equal(t, 1, oLogger.FilterMessageSnippet("failed to read data from the upstream response").Len())

	err = r.Body.Close()
	require.NoError(t, err)

	stopCh <- struct{}{}
	wg.Wait()
}
