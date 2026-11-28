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
	"context"
	"time"
)

// ConnectError says that a command was not run, because no SSH session with the machine could be set up. Its message
// is the message of the error it wraps.
type ConnectError struct {
	Err error
}

func (e *ConnectError) Error() string {
	return e.Err.Error()
}

func (e *ConnectError) Unwrap() error {
	return e.Err
}

const (
	// DefaultConnectTimeout is how long a Runner waits for a machine to accept a connection and a login.
	DefaultConnectTimeout = 10 * time.Second
	// DefaultCommandTimeout is how long a Runner waits for a command. The commands that the operator runs download
	// etcd and start it, which takes a few minutes on a slow network.
	DefaultCommandTimeout = 10 * time.Minute
)

// noDeadline removes a deadline from a connection.
var noDeadline time.Time

// Runner runs commands on machines over SSH. A machine that does not answer costs it at most ConnectTimeout, and a
// command that does not finish at most CommandTimeout.
type Runner struct {
	ConnectTimeout time.Duration
	CommandTimeout time.Duration
}

// NewRunner returns a Runner with the default time limits.
func NewRunner() *Runner {
	return &Runner{
		ConnectTimeout: DefaultConnectTimeout,
		CommandTimeout: DefaultCommandTimeout,
	}
}

// Run connects to a machine and runs one command on it. It returns a ConnectError when it could not connect, and
// another error when the command ran and failed, or did not finish in time.
func (r *Runner) Run(ctx context.Context, privateKey []byte, address string, port int, command string) error {
	connectCtx, cancelConnect := context.WithTimeout(ctx, r.ConnectTimeout)
	defer cancelConnect()
	client, closer, err := StartSSHConnection(connectCtx, privateKey, address, port)
	if err != nil {
		return &ConnectError{Err: err}
	}
	defer closer()

	commandCtx, cancelCommand := context.WithTimeout(ctx, r.CommandTimeout)
	defer cancelCommand()
	return RunCommandOverSSHSession(commandCtx, client, command)
}
