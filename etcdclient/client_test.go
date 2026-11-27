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

package etcdclient_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/kkohtaka/kubernetesimal/etcdclient"
)

// certificate returns a serving certificate that is valid for 127.0.0.1, and a client configuration that trusts it.
// Both come from the test server of the standard library, which is not started.
func certificate(t *testing.T) (tls.Certificate, *tls.Config) {
	t.Helper()
	server := httptest.NewUnstartedServer(nil)
	server.StartTLS()
	t.Cleanup(server.Close)

	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	return server.TLS.Certificates[0], &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
}

func startHealthServer(t *testing.T, status int) (string, *tls.Config) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)

	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	return strings.TrimPrefix(server.URL, "https://"), &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
}

// cluster answers the two calls of the etcd API that the operator makes besides the health check.
type cluster struct {
	etcdserverpb.UnimplementedClusterServer
	etcdserverpb.UnimplementedMaintenanceServer

	members []*etcdserverpb.Member
}

func (c *cluster) MemberList(context.Context, *etcdserverpb.MemberListRequest) (*etcdserverpb.MemberListResponse, error) {
	return &etcdserverpb.MemberListResponse{Header: &etcdserverpb.ResponseHeader{}, Members: c.members}, nil
}

func (c *cluster) Status(context.Context, *etcdserverpb.StatusRequest) (*etcdserverpb.StatusResponse, error) {
	return &etcdserverpb.StatusResponse{Header: &etcdserverpb.ResponseHeader{}, Version: "3.5.1"}, nil
}

// startCluster serves the etcd API over TLS. With a nil cluster, every call is answered as not implemented.
func startCluster(t *testing.T, c *cluster) (string, *tls.Config) {
	t.Helper()
	serving, clientConfig := certificate(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer(grpc.Creds(credentials.NewServerTLSFromCert(&serving)))
	if c != nil {
		etcdserverpb.RegisterClusterServer(server, c)
		etcdserverpb.RegisterMaintenanceServer(server, c)
	} else {
		etcdserverpb.RegisterClusterServer(server, &etcdserverpb.UnimplementedClusterServer{})
		etcdserverpb.RegisterMaintenanceServer(server, &etcdserverpb.UnimplementedMaintenanceServer{})
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	return listener.Addr().String(), clientConfig
}

func TestHealthIsTrueWhenTheEndpointAnswersOK(t *testing.T) {
	address, tlsConfig := startHealthServer(t, http.StatusOK)

	healthy, err := etcdclient.New().Health(context.Background(), address, tlsConfig)
	require.NoError(t, err)
	assert.True(t, healthy)
}

func TestHealthIsFalseWhenTheEndpointAnswersAnError(t *testing.T) {
	address, tlsConfig := startHealthServer(t, http.StatusServiceUnavailable)

	healthy, err := etcdclient.New().Health(context.Background(), address, tlsConfig)
	require.NoError(t, err)
	assert.False(t, healthy)
}

func TestHealthIsFalseWhenNothingAnswers(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	healthy, err := etcdclient.New().Health(context.Background(), address, nil)
	require.NoError(t, err)
	assert.False(t, healthy)
}

func TestMemberListReturnsTheMembersOfTheCluster(t *testing.T) {
	address, tlsConfig := startCluster(t, &cluster{members: []*etcdserverpb.Member{
		{ID: 1, Name: "member-a", ClientURLs: []string{"https://10.0.0.1:2379"}},
		{ID: 2, Name: "member-b", ClientURLs: []string{"https://10.0.0.2:2379", "https://member-b:2379"}},
	}})

	members, err := etcdclient.New().MemberList(context.Background(), address, tlsConfig)
	require.NoError(t, err)
	assert.Equal(t, []etcdclient.Member{
		{Name: "member-a", ClientURLs: []string{"https://10.0.0.1:2379"}},
		{Name: "member-b", ClientURLs: []string{"https://10.0.0.2:2379", "https://member-b:2379"}},
	}, members)
}

func TestMemberListReportsAClusterThatDoesNotAnswerTheCall(t *testing.T) {
	address, tlsConfig := startCluster(t, nil)

	_, err := etcdclient.New().MemberList(context.Background(), address, tlsConfig)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "unable to list etcd members: "), "got %v", err)
}

func TestMemberStatusSucceedsWhenTheMemberAnswers(t *testing.T) {
	address, tlsConfig := startCluster(t, &cluster{})

	err := etcdclient.New().MemberStatus(context.Background(), "https://"+address, tlsConfig)
	assert.NoError(t, err)
}

func TestMemberStatusReportsAMemberThatDoesNotAnswerTheCall(t *testing.T) {
	address, tlsConfig := startCluster(t, nil)

	err := etcdclient.New().MemberStatus(context.Background(), "https://"+address, tlsConfig)
	assert.Error(t, err)
}
