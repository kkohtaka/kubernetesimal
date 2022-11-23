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

	"golang.org/x/crypto/ssh"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func StartSSHConnection(_ context.Context, privateKey []byte, address string, port int) (*ssh.Client, func(), error) {
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

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", address, port), config)
	if err != nil {
		return nil, nil, fmt.Errorf("could not dial: %w", err)
	}
	return client, func() { client.Close() }, nil
}

func RunCommandOverSSHSession(ctx context.Context, client *ssh.Client, cmd string) error {
	logger := log.FromContext(ctx)

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("could not create SSH session: %w", err)
	}
	defer session.Close()

	var out, errOut bytes.Buffer
	session.Stdout = &out
	session.Stderr = &errOut
	if err := session.Run(cmd); err != nil {
		logger.Error(err, "Could not complete a command", "cmd", cmd, "errOut", errOut.String())
		return fmt.Errorf("unable to complete a command: %w", err)
	}
	logger.V(4).Info("Succeeded in completing a command", "cmd", cmd, "out", out.String())
	return nil
}
