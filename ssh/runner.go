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

// Runner runs commands on machines over SSH.
type Runner struct{}

// NewRunner returns a Runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Run connects to a machine and runs one command on it. It returns a ConnectError when it could not connect, and
// another error when the command ran and failed.
func (r *Runner) Run(ctx context.Context, privateKey []byte, address string, port int, command string) error {
	client, closer, err := StartSSHConnection(ctx, privateKey, address, port)
	if err != nil {
		return &ConnectError{Err: err}
	}
	defer closer()

	return RunCommandOverSSHSession(ctx, client, command)
}
