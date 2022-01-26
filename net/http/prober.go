package http

import (
	"context"
	"crypto/tls"
	nethttp "net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Prober struct {
	url                string
	expectedStatusCode int
	interval           time.Duration
	timeout            time.Duration
	tlsConfig          *tls.Config
}

func WithExpectedStatusCode(code int) func(*Prober) {
	return func(p *Prober) {
		p.expectedStatusCode = code
	}
}

func WithInterval(interval time.Duration) func(*Prober) {
	return func(p *Prober) {
		p.interval = interval
	}
}

func WithTimeout(timeout time.Duration) func(*Prober) {
	return func(p *Prober) {
		p.timeout = timeout
	}
}

func WithTLSConfig(tlsConfig *tls.Config) func(*Prober) {
	return func(p *Prober) {
		p.tlsConfig = tlsConfig
	}
}

func NewProber(
	url string,
	opts ...func(p *Prober),
) *Prober {
	p := &Prober{
		url:                url,
		expectedStatusCode: 200,
		interval:           5 * time.Second,
		timeout:            2 * time.Second,
	}
	for _, fn := range opts {
		fn(p)
	}
	return p
}

func (p *Prober) Once(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, p.url, nil)
	if err != nil {
		return false, err
	}
	client := *nethttp.DefaultClient
	if p.tlsConfig != nil {
		client.Transport = &nethttp.Transport{
			TLSClientConfig: p.tlsConfig,
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		log.FromContext(ctx).Info(
			"Probing was failed.",
			"url", p.url,
			"reason", err,
		)
		return false, nil
	}
	defer resp.Body.Close()
	return resp.StatusCode == p.expectedStatusCode, nil
}
