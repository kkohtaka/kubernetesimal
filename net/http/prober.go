/*
MIT License

Copyright (c) 2022 Kazumasa Kohtaka

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

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
