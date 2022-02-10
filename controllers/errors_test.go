package controllers_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kkohtaka/kubernetesimal/controllers"
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
			target: controllers.NewRequeueError("a RequeueError"),
			want:   true,
		},
		{
			name:   "not a RequeueError",
			target: fmt.Errorf("not a RequeueError"),
			want:   false,
		},
		{
			name:   "an error wrapping a RequeueError",
			target: fmt.Errorf("not a RequeueError: %w", controllers.NewRequeueError("a RequeueError")),
			want:   true,
		},
		{
			name:   "a RequeueError wrapping an error",
			target: controllers.NewRequeueError("a RequeueError").Wrap(fmt.Errorf("not a RequeueError")),
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, controllers.ShouldRequeue(tt.target))
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
			target: controllers.NewRequeueError("a RequeueError"),
			want:   0,
		},
		{
			name:   "a RequeueError with a delay",
			target: controllers.NewRequeueError("a RequeueError").WithDelay(5 * time.Second),
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
				controllers.NewRequeueError("a RequeueError").WithDelay(5*time.Second),
			),
			want: 5 * time.Second,
		},
		{
			name: "a RequeueError wrapping an error",
			target: controllers.NewRequeueError("a RequeueError").
				Wrap(fmt.Errorf("not a RequeueError")).
				WithDelay(5 * time.Second),
			want: 5 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, controllers.GetDelay(tt.target))
		})
	}
}
