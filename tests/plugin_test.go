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

	"github.com/roadrunner-server/config/v4"
	"github.com/roadrunner-server/endure/v2"
	httpPlugin "github.com/roadrunner-server/http/v4"
	"github.com/roadrunner-server/logger/v4"
	"github.com/roadrunner-server/sendremotefile/v4"
	"github.com/roadrunner-server/server/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSendremotefileInit(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-with-sendremotefile.yaml",
		Prefix:  "rr",
	}

	err := cont.RegisterAll(
		cfg,
		&logger.Plugin{},
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
	stopCh <- struct{}{}
	wg.Wait()
}

func TestSendremotefileDisabled(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-with-disabled-sendremotefile.yaml",
		Prefix:  "rr",
	}

	err := cont.RegisterAll(
		cfg,
		&logger.Plugin{},
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
	t.Run("middlewareDisabledCheck", middlewareDisabledCheck)

	stopCh <- struct{}{}
	wg.Wait()
}

func middlewareDisabledCheck(t *testing.T) {
	r, err := http.DefaultClient.Get("http://127.0.0.1:18953/remote-file")
	require.NoError(t, err)

	assert.Equal(t, 200, r.StatusCode)
	assert.Equal(t, "http://127.0.0.1:18953/file", r.Header.Get("X-Sendremotefile"))

	err = r.Body.Close()
	require.NoError(t, err)
}

func TestSendremotefileFileStream(t *testing.T) {
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
	t.Run("remoteFileCheck", remoteFileCheck)
	t.Run("localFileCheck", localFileCheck(oLogger))
	t.Run("remoteFileNotFoundCheck", remoteFileNotFoundCheck(oLogger))
	t.Run("remoteFileTimeoutCheck", remoteFileTimeoutCheck(oLogger))

	stopCh <- struct{}{}
	wg.Wait()
}

func remoteFileCheck(t *testing.T) {
	r, err := http.DefaultClient.Get("http://127.0.0.1:18953/remote-file")
	require.NoError(t, err)

	b, err := io.ReadAll(r.Body)
	require.NoError(t, err)

	file, err := os.Open("./data/1MB.jpg")
	require.NoError(t, err)
	defer file.Close()
	fs, err := file.Stat()
	require.NoError(t, err)

	assert.Equal(t, int(fs.Size()), len(b))
	assert.Equal(t, 200, r.StatusCode)
	assert.Equal(t, "", r.Header.Get("X-Sendremotefile"))
	assert.Equal(t, "attachment; filename=1MB.jpg", r.Header.Get("Content-Disposition"))
	assert.Equal(t, "application/octet-stream", r.Header.Get("Content-Type"))

	err = r.Body.Close()
	require.NoError(t, err)
}

func localFileCheck(oLogger *mocklogger.ObservedLogs) func(t *testing.T) {
	return func(t *testing.T) {
		r, err := http.DefaultClient.Get("http://127.0.0.1:18953/local-file")
		require.NoError(t, err)
		defer r.Body.Close()

		assert.Equal(t, 404, r.StatusCode)
		assert.Equal(t, "", r.Header.Get("X-Sendremotefile"))
		assert.Equal(t, 1, oLogger.FilterMessageSnippet("header value must start with http").Len())

		err = r.Body.Close()
		require.NoError(t, err)
	}
}

func remoteFileNotFoundCheck(oLogger *mocklogger.ObservedLogs) func(t *testing.T) {
	return func(t *testing.T) {
		r, err := http.DefaultClient.Get("http://127.0.0.1:18953/remote-file-not-found")
		require.NoError(t, err)
		defer r.Body.Close()

		assert.Equal(t, 400, r.StatusCode)
		assert.Equal(t, "", r.Header.Get("X-Sendremotefile"))
		assert.Equal(t, 1, oLogger.FilterMessageSnippet("invalid upstream response status code").Len())

		err = r.Body.Close()
		require.NoError(t, err)
	}
}

func remoteFileTimeoutCheck(oLogger *mocklogger.ObservedLogs) func(t *testing.T) {
	return func(t *testing.T) {
		r, err := http.DefaultClient.Get("http://127.0.0.1:18953/remote-file-timeout")
		require.NoError(t, err)

		assert.Equal(t, 408, r.StatusCode)
		assert.Equal(t, "", r.Header.Get("X-Sendremotefile"))

		err = r.Body.Close()
		require.NoError(t, err)
	}
}
