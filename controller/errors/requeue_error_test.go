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

package errors_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kkohtaka/kubernetesimal/controller/errors"
	"github.com/stretchr/testify/assert"
)

func TestShouldRequeue(t *testing.T) {
	tests := []struct {
		name   string
		target error
		want   bool
	}{
		{
			name:   "a RequeueError",
			target: errors.NewRequeueError("a RequeueError"),
			want:   true,
		},
		{
			name:   "not a RequeueError",
			target: fmt.Errorf("not a RequeueError"),
			want:   false,
		},
		{
			name:   "an error wrapping a RequeueError",
			target: fmt.Errorf("not a RequeueError: %w", errors.NewRequeueError("a RequeueError")),
			want:   true,
		},
		{
			name:   "a RequeueError wrapping an error",
			target: errors.NewRequeueError("a RequeueError").Wrap(fmt.Errorf("not a RequeueError")),
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, errors.ShouldRequeue(tt.target))
		})
	}
}

func TestGetDelay(t *testing.T) {
	tests := []struct {
		name   string
		target error
		want   time.Duration
	}{
		{
			name:   "a RequeueError",
			target: errors.NewRequeueError("a RequeueError"),
			want:   0,
		},
		{
			name:   "a RequeueError with a delay",
			target: errors.NewRequeueError("a RequeueError").WithDelay(5 * time.Second),
			want:   5 * time.Second,
		},
		{
			name:   "not a RequeueError",
			target: fmt.Errorf("not a RequeueError"),
			want:   0,
		},
		{
			name: "an error wrapping a RequeueError",
			target: fmt.Errorf("not a RequeueError: %w",
				errors.NewRequeueError("a RequeueError").WithDelay(5*time.Second),
			),
			want: 5 * time.Second,
		},
		{
			name: "a RequeueError wrapping an error",
			target: errors.NewRequeueError("a RequeueError").
				Wrap(fmt.Errorf("not a RequeueError")).
				WithDelay(5 * time.Second),
			want: 5 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, errors.GetDelay(tt.target))
		})
	}
}
