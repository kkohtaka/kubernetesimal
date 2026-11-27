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

// Package etcdclient is how the operator talks to etcd. The controllers use the interface, so that tests can replace
// etcd.
package etcdclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kkohtaka/kubernetesimal/net/http"
)

const (
	defaultRequestTimeout = 5 * time.Second

	defaultMemberStatusTimeout = time.Second
)

// Member is a member of an etcd cluster, as the cluster lists it.
type Member struct {
	Name       string
	ClientURLs []string
}

// Interface is what the operator asks of etcd.
type Interface interface {
	// Health reports whether the health endpoint at an address, given as host:port, answers that etcd is healthy.
	// An address that does not answer is not healthy, and is not an error.
	Health(ctx context.Context, address string, tlsConfig *tls.Config) (bool, error)

	// MemberList returns the members of the cluster that is reached at an address, given as host:port.
	MemberList(ctx context.Context, address string, tlsConfig *tls.Config) ([]Member, error)

	// MemberStatus asks the member at a client URL for its status, and returns an error when it does not answer.
	MemberStatus(ctx context.Context, clientURL string, tlsConfig *tls.Config) error
}

// Client talks to etcd over the network.
type Client struct{}

var _ Interface = (*Client)(nil)

// New returns a Client.
func New() *Client {
	return &Client{}
}

func (c *Client) Health(ctx context.Context, address string, tlsConfig *tls.Config) (bool, error) {
	return http.NewProber(
		fmt.Sprintf("https://%s/health", address),
		http.WithTLSConfig(tlsConfig),
	).Once(ctx)
}

func (c *Client) MemberList(ctx context.Context, address string, tlsConfig *tls.Config) ([]Member, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints: []string{
			fmt.Sprintf("https://%s", address),
		},
		TLS: tlsConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create an etcd client: %w", err)
	}

	listMemberCtx, listMemberCancel := context.WithTimeout(ctx, defaultRequestTimeout)
	resp, err := client.MemberList(listMemberCtx)
	listMemberCancel()
	if err != nil {
		return nil, fmt.Errorf("unable to list etcd members: %w", err)
	}
	log.FromContext(ctx).V(4).Info("List etcd members.", "members", resp.Members)

	members := make([]Member, 0, len(resp.Members))
	for _, member := range resp.Members {
		members = append(members, Member{Name: member.Name, ClientURLs: member.GetClientURLs()})
	}
	return members, nil
}

func (c *Client) MemberStatus(ctx context.Context, clientURL string, tlsConfig *tls.Config) error {
	logger := log.FromContext(ctx)

	client, err := clientv3.New(clientv3.Config{
		Endpoints: []string{clientURL},
		TLS:       tlsConfig,
	})
	if err != nil {
		logger.Error(err, "Creating an etcd client to check member's health was failed.")
		return err
	}

	statusCtx, statusCancel := context.WithTimeout(ctx, defaultMemberStatusTimeout)
	defer statusCancel()
	if _, err := client.Status(statusCtx, clientURL); err != nil {
		logger.V(4).Error(err, "Checking a status of an etcd member was failed.")
		return err
	}
	return nil
}
