package sendremotefile

import (
	"context"
	"net"
	"net/http"
	"time"
)

type client struct {
	inner *http.Client
	url   string
}

type connection struct {
	net.Conn
	timeout time.Duration
}

func NewClient(url string, timeout time.Duration) *client {
	return &client{
		inner: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, network, addr)
					if err != nil {
						return nil, err
					}

					return &connection{
						Conn:    conn,
						timeout: timeout,
					}, nil
				},
				TLSHandshakeTimeout:   timeout,
				ResponseHeaderTimeout: timeout,
			},
		},
		url: url,
	}
}

func (c *client) Request() (*http.Response, error) {
	req, err := http.NewRequest("GET", c.url, nil)
	if err != nil {
		return nil, err
	}

	return c.inner.Do(req)
}

func (c *connection) Read(b []byte) (int, error) {
	err := c.Conn.SetReadDeadline(time.Now().Add(c.timeout))
	if err != nil {
		return 0, err
	}

	return c.Conn.Read(b)
}
