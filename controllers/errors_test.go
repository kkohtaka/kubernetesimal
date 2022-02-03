package controllers_test

import (
	"fmt"
	"testing"

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
