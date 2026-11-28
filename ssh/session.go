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

package ssh

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strconv"

	"golang.org/x/crypto/ssh"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// StartSSHConnection connects to a machine and logs in. It gives up when the context ends, and the context should
// have a deadline: without one, a machine that accepts the connection and then says nothing is waited for forever.
func StartSSHConnection(ctx context.Context, privateKey []byte, address string, port int) (*ssh.Client, func(), error) {
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: "fedora",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	target := net.JoinHostPort(address, strconv.Itoa(port))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, nil, fmt.Errorf("could not dial: %w", err)
	}
	// The handshake reads from the connection, which the deadline of the context has to end as well.
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			_ = conn.Close()
			return nil, nil, fmt.Errorf("could not dial: %w", err)
		}
	}
	clientConn, channels, requests, err := ssh.NewClientConn(conn, target, config)
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("could not dial: %w", err)
	}
	// The deadline was for logging in. How long a command may take is up to RunCommandOverSSHSession.
	if err := conn.SetDeadline(noDeadline); err != nil {
		_ = clientConn.Close()
		return nil, nil, fmt.Errorf("could not dial: %w", err)
	}

	client := ssh.NewClient(clientConn, channels, requests)
	return client, func() { client.Close() }, nil
}

// RunCommandOverSSHSession runs a command and waits for it. When the context ends first, it closes the connection,
// which is the only way to stop waiting, and returns the error of the context.
func RunCommandOverSSHSession(ctx context.Context, client *ssh.Client, cmd string) error {
	logger := log.FromContext(ctx)

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("could not create SSH session: %w", err)
	}
	defer func() { _ = session.Close() }()

	var out, errOut bytes.Buffer
	session.Stdout = &out
	session.Stderr = &errOut

	done := make(chan error, 1)
	go func() { done <- session.Run(cmd) }()

	select {
	case err = <-done:
	case <-ctx.Done():
		_ = client.Close()
		// Closing the connection ends the command. What it returns then says nothing new.
		if runErr := <-done; runErr != nil {
			logger.V(4).Info("The command ended when its connection was closed.", "reason", runErr.Error())
		}
		err = ctx.Err()
	}
	if err != nil {
		logger.Error(err, "Could not complete a command", "cmd", cmd, "errOut", errOut.String())
		return fmt.Errorf("unable to complete a command: %w", err)
	}
	logger.V(4).Info("Succeeded in completing a command", "cmd", cmd, "out", out.String())
	return nil
}
