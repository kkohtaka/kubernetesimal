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
	logger.Info("Succeeded in completing a command", "cmd", cmd, "out", out.String())
	return nil
}
