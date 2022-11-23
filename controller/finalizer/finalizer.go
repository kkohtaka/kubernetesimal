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

package finalizer

import (
	"context"
	"time"

	"github.com/kkohtaka/kubernetesimal/controller/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	finalizerName = "kubernetesimal.kkohtaka.org/finalizer"
)

func HasFinalizer(o client.Object) bool {
	return controllerutil.ContainsFinalizer(o, finalizerName)
}

func SetFinalizer(ctx context.Context, c client.Client, o client.Object) error {
	newO := o.DeepCopyObject().(client.Object)
	controllerutil.AddFinalizer(newO, finalizerName)
	return c.Patch(ctx, newO, client.MergeFrom(o))
}

func UnsetFinalizer(ctx context.Context, c client.Client, o client.Object) error {
	newO := o.DeepCopyObject().(client.Object)
	controllerutil.RemoveFinalizer(newO, finalizerName)
	return c.Patch(ctx, newO, client.MergeFrom(o))
}

func FinalizeSecret(
	ctx context.Context,
	client client.Client,
	namespace, name string,
) error {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues(
		"object", name,
		"resource", "corev1.Secret",
	))
	return FinalizeObject(ctx, client, namespace, name, &corev1.Secret{})
}

func FinalizeObject(
	ctx context.Context,
	c client.Client,
	namespace, name string,
	obj client.Object,
) error {
	logger := log.FromContext(ctx)

	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	if err := c.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if obj.GetDeletionTimestamp().IsZero() {
		if err := c.Delete(ctx, obj, &client.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		logger.Info("The object has started to be deleted.")
	}
	return errors.NewRequeueError("waiting for an object deleted").WithDelay(5 * time.Second)
}
