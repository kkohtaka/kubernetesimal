package controllers

import (
	"context"
	"time"

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

func addFinalizer(ctx context.Context, c client.Client, o client.Object, finalizer string) error {
	newO := o.DeepCopyObject().(client.Object)
	controllerutil.AddFinalizer(newO, finalizerName)
	return c.Patch(ctx, newO, client.MergeFrom(o))
}

func removeFinalizer(ctx context.Context, c client.Client, o client.Object, finalizer string) error {
	newO := o.DeepCopyObject().(client.Object)
	controllerutil.RemoveFinalizer(newO, finalizerName)
	return c.Patch(ctx, newO, client.MergeFrom(o))
}

func finalizeSecret(
	ctx context.Context,
	client client.Client,
	namespace, name string,
) error {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues(
		"object", name,
		"resource", "corev1.Secret",
	))
	return finalizeObject(ctx, client, namespace, name, &corev1.Secret{})
}

func finalizeObject(
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
	return NewRequeueError("waiting for an object deleted").WithDelay(5 * time.Second)
}
