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

package errors

import (
	"errors"
	"fmt"
	"time"
)

type RequeueError struct {
	msg string
	err error

	delay time.Duration
}

var _ error = (*RequeueError)(nil)

func NewRequeueError(msg string) *RequeueError {
	return &RequeueError{
		msg: msg,
	}
}

func (re *RequeueError) Error() string {
	if re.err == nil {
		return re.msg
	}
	return fmt.Sprintf("%s: %s", re.msg, re.err)
}

func (re *RequeueError) Unwrap() error {
	return re.err
}

func (re *RequeueError) WithDelay(delay time.Duration) *RequeueError {
	newErr := *re
	newErr.delay = delay
	return &newErr
}

func (re *RequeueError) Wrap(err error) *RequeueError {
	newErr := *re
	newErr.err = err
	return &newErr
}

func ShouldRequeue(err error) bool {
	var re *RequeueError
	if ok := errors.As(err, &re); !ok {
		return false
	}
	return true
}

func GetDelay(err error) time.Duration {
	var re *RequeueError
	if ok := errors.As(err, &re); ok {
		return re.delay
	}
	return 0
}
