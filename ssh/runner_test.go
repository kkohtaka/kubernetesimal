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

package ssh_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	kssh "github.com/kkohtaka/kubernetesimal/ssh"
)

// server is an SSH server that accepts one public key and answers every command with a fixed exit status.
type server struct {
	host string
	port int

	// hang, when it is not nil, keeps the server from answering a command until it is closed.
	hang chan struct{}

	mu       sync.Mutex
	users    []string
	commands []string
}

func (s *server) recorded() (users, commands []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.users...), append([]string(nil), s.commands...)
}

func startServer(t *testing.T, authorizedKey []byte, exitStatus uint32) *server {
	t.Helper()

	allowed, _, _, _, err := ssh.ParseAuthorizedKey(authorizedKey)
	require.NoError(t, err)

	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	hostSigner, err := ssh.NewSignerFromKey(hostKey)
	require.NoError(t, err)

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if !bytes.Equal(key.Marshal(), allowed.Marshal()) {
				return nil, errors.New("unknown public key")
			}
			return &ssh.Permissions{}, nil
		},
	}
	config.AddHostKey(hostSigner)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	s := &server{host: addr.IP.String(), port: addr.Port}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go s.serve(conn, config, exitStatus)
		}
	}()
	return s
}

func (s *server) serve(conn net.Conn, config *ssh.ServerConfig, exitStatus uint32) {
	defer func() { _ = conn.Close() }()

	serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(requests)

	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "only sessions are served")
			continue
		}
		channel, channelRequests, err := newChannel.Accept()
		if err != nil {
			return
		}
		for request := range channelRequests {
			if request.Type != "exec" {
				_ = request.Reply(false, nil)
				continue
			}
			var payload struct{ Command string }
			if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
				_ = request.Reply(false, nil)
				continue
			}

			s.mu.Lock()
			s.users = append(s.users, serverConn.User())
			s.commands = append(s.commands, payload.Command)
			s.mu.Unlock()

			_ = request.Reply(true, nil)
			if s.hang != nil {
				<-s.hang
			}
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{exitStatus}))
			_ = channel.Close()
		}
	}
}

func newKeyPair(t *testing.T) (privateKey, publicKey []byte) {
	t.Helper()
	privateKey, publicKey, err := kssh.GenerateKeyPair(func(o *kssh.KeyPairOption) { o.BitSize = 2048 })
	require.NoError(t, err)
	return privateKey, publicKey
}

func TestRunnerRunsACommandOnTheMachine(t *testing.T) {
	privateKey, publicKey := newKeyPair(t)
	s := startServer(t, publicKey, 0)

	err := kssh.NewRunner().Run(context.Background(), privateKey, s.host, s.port, "sudo /opt/bin/start-cluster.sh")
	require.NoError(t, err)

	users, commands := s.recorded()
	assert.Equal(t, []string{"fedora"}, users)
	assert.Equal(t, []string{"sudo /opt/bin/start-cluster.sh"}, commands)
}

func TestRunnerReportsACommandThatFails(t *testing.T) {
	privateKey, publicKey := newKeyPair(t)
	s := startServer(t, publicKey, 1)

	err := kssh.NewRunner().Run(context.Background(), privateKey, s.host, s.port, "false")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to complete a command")

	var connectErr *kssh.ConnectError
	assert.False(t, errors.As(err, &connectErr), "a command that ran and failed is not a failure to connect")

	_, commands := s.recorded()
	assert.Equal(t, []string{"false"}, commands)
}

func TestRunnerReportsAMachineThatDoesNotAnswer(t *testing.T) {
	privateKey, _ := newKeyPair(t)

	// Take a port and release it, so that nothing listens on it.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	require.NoError(t, listener.Close())

	err = kssh.NewRunner().Run(context.Background(), privateKey, addr.IP.String(), addr.Port, "true")
	require.Error(t, err)

	var connectErr *kssh.ConnectError
	require.True(t, errors.As(err, &connectErr), "got %v", err)
	assert.Contains(t, err.Error(), "could not dial")
	assert.Equal(t, connectErr.Err.Error(), err.Error(), "the message is the one of the underlying error")
}

func TestRunnerReportsAMachineThatRefusesTheKey(t *testing.T) {
	privateKey, _ := newKeyPair(t)
	_, otherPublicKey := newKeyPair(t)
	s := startServer(t, otherPublicKey, 0)

	err := kssh.NewRunner().Run(context.Background(), privateKey, s.host, s.port, "true")
	require.Error(t, err)

	var connectErr *kssh.ConnectError
	require.True(t, errors.As(err, &connectErr), "got %v", err)
	assert.Contains(t, err.Error(), "could not dial")

	_, commands := s.recorded()
	assert.Empty(t, commands)
}

func TestRunnerReportsAPrivateKeyItCannotRead(t *testing.T) {
	err := kssh.NewRunner().Run(context.Background(), []byte("not a key"), "127.0.0.1", 22, "true")
	require.Error(t, err)

	var connectErr *kssh.ConnectError
	require.True(t, errors.As(err, &connectErr), "got %v", err)
	assert.Contains(t, err.Error(), "unable to parse private key")
}

// startSilentListener accepts connections and never says anything on them, like a machine whose SSH server hangs.
func startSilentListener(t *testing.T) (string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var (
		mu    sync.Mutex
		conns []net.Conn
	)
	t.Cleanup(func() {
		_ = listener.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range conns {
			_ = conn.Close()
		}
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()

	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	return addr.IP.String(), addr.Port
}

// within fails the test when f takes longer than the limit, and returns what f returned.
func within(t *testing.T, limit time.Duration, f func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("still running after %s", limit)
		return nil
	}
}

func TestRunnerGivesUpOnAMachineThatDoesNotCompleteTheConnection(t *testing.T) {
	privateKey, _ := newKeyPair(t)
	host, port := startSilentListener(t)

	runner := kssh.NewRunner()
	runner.ConnectTimeout = 300 * time.Millisecond

	err := within(t, 5*time.Second, func() error {
		return runner.Run(context.Background(), privateKey, host, port, "true")
	})
	require.Error(t, err)

	var connectErr *kssh.ConnectError
	require.True(t, errors.As(err, &connectErr), "got %v", err)
}

func TestRunnerGivesUpOnACommandThatDoesNotFinish(t *testing.T) {
	privateKey, publicKey := newKeyPair(t)
	s := startServer(t, publicKey, 0)
	s.hang = make(chan struct{})
	t.Cleanup(func() { close(s.hang) })

	runner := kssh.NewRunner()
	runner.CommandTimeout = 300 * time.Millisecond

	err := within(t, 5*time.Second, func() error {
		return runner.Run(context.Background(), privateKey, s.host, s.port, "sleep infinity")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to complete a command")
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	var connectErr *kssh.ConnectError
	assert.False(t, errors.As(err, &connectErr), "the machine was reached, so this is not a failure to connect")

	_, commands := s.recorded()
	assert.Equal(t, []string{"sleep infinity"}, commands)
}

func TestRunnerStopsWaitingForACommandWhenItIsCancelled(t *testing.T) {
	privateKey, publicKey := newKeyPair(t)
	s := startServer(t, publicKey, 0)
	s.hang = make(chan struct{})
	t.Cleanup(func() { close(s.hang) })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	err := within(t, 5*time.Second, func() error {
		return kssh.NewRunner().Run(ctx, privateKey, s.host, s.port, "sleep infinity")
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunnerHasTimeLimitsUnlessToldOtherwise(t *testing.T) {
	runner := kssh.NewRunner()
	assert.Positive(t, runner.ConnectTimeout)
	assert.Positive(t, runner.CommandTimeout)
	assert.Greater(t, runner.CommandTimeout, runner.ConnectTimeout)
}
