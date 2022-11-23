package etcd

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/controller/errors"
	"github.com/kkohtaka/kubernetesimal/controller/finalizer"
	k8s_etcdnodedeployment "github.com/kkohtaka/kubernetesimal/k8s/etcdnodedeployment"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
)

func newEtcdNodeDeploymentName(e client.Object) string {
	return e.GetName()
}

func newEtcdNodeDeploymentSelector(e client.Object) *metav1.LabelSelector {
	return metav1.SetAsLabelSelector(newEtcdNodeTemplateSpecLabels(e))
}

func newEtcdNodeTemplateSpecLabels(e client.Object) map[string]string {
	return map[string]string{
		"app.kubernetes.io/part-of":    e.GetName(),
		"app.kubernetes.io/managed-by": "etcd.kubernetesimal.kkohtaka.org",
	}
}

func reconcileEtcdNodeDeployment(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	e client.Object,
	spec *kubernetesimalv1alpha1.EtcdSpec,
	status *kubernetesimalv1alpha1.EtcdStatus,
) (*kubernetesimalv1alpha1.EtcdNodeDeployment, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileEtcdNodeDeployment")
	defer span.End()
	logger := log.FromContext(ctx)

	if !status.IsReadyOnce() {
		// Create a single-node cluster before it becomes ready once.
		template := kubernetesimalv1alpha1.EtcdNodeTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: newEtcdNodeTemplateSpecLabels(e),
			},
			Spec: kubernetesimalv1alpha1.EtcdNodeSpec{
				Version:              *spec.Version,
				CACertificateRef:     *status.CACertificateRef,
				CAPrivateKeyRef:      *status.CAPrivateKeyRef,
				ClientCertificateRef: *status.ClientCertificateRef,
				ClientPrivateKeyRef:  *status.ClientPrivateKeyRef,
				SSHPrivateKeyRef:     *status.SSHPrivateKeyRef,
				SSHPublicKeyRef:      *status.SSHPublicKeyRef,
				ServiceRef:           *status.ServiceRef,
				AsFirstNode:          true,
			},
		}

		// If a corresponding deployment exists and the spec should be changed, scale its replicas to zero before
		// changing the spec.
		if _, deployment, err := k8s_etcdnodedeployment.Create(
			ctx,
			c,
			newEtcdNodeDeploymentName(e),
			e.GetNamespace(),
			k8s_etcdnodedeployment.WithReplicas(1),
			k8s_etcdnodedeployment.WithSelector(newEtcdNodeDeploymentSelector(e)),
			k8s_etcdnodedeployment.WithTemplate(&template),
		); err != nil {
			if apierrors.IsAlreadyExists(err) {
				var deployment kubernetesimalv1alpha1.EtcdNodeDeployment
				deployment.Name = newEtcdNodeDeploymentName(e)
				deployment.Namespace = e.GetNamespace()
				if err := c.Get(ctx, client.ObjectKeyFromObject(&deployment), &deployment); err != nil {
					return nil, fmt.Errorf("unable to get EtcdNodeDeployment: %w", err)
				}

				if !apiequality.Semantic.DeepEqual(template, deployment.Spec.Template) {
					logger.Info("The desired spec of Etcd was changed while building the first single-node cluster.")
					if _, updatedDeployment, err := k8s_etcdnodedeployment.Reconcile(
						ctx,
						c,
						newEtcdNodeDeploymentName(e),
						e.GetNamespace(),
						k8s_etcdnodedeployment.WithReplicas(0),
						k8s_etcdnodedeployment.WithSelector(newEtcdNodeDeploymentSelector(e)),
						k8s_etcdnodedeployment.WithTemplate(&template),
					); err != nil {
						return nil, fmt.Errorf("unable to scale EtcdNodeDeployment to zero: %w", err)
					} else {
						updatedDeployment.DeepCopyInto(&deployment)
					}
					logger.Info("EtcdNodeDeployment was scaled to zero.")
				}
				return &deployment, nil
			}
			return nil, fmt.Errorf("unable to create the first EtcdNodeDeployment: %w", err)
		} else {
			return deployment, nil
		}
	}

	var replicas int32 = 1
	if spec.Replicas != nil {
		replicas = *spec.Replicas
	}
	template := kubernetesimalv1alpha1.EtcdNodeTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: newEtcdNodeTemplateSpecLabels(e),
		},
		Spec: kubernetesimalv1alpha1.EtcdNodeSpec{
			Version:              *spec.Version,
			CACertificateRef:     *status.CACertificateRef,
			CAPrivateKeyRef:      *status.CAPrivateKeyRef,
			ClientCertificateRef: *status.ClientCertificateRef,
			ClientPrivateKeyRef:  *status.ClientPrivateKeyRef,
			SSHPrivateKeyRef:     *status.SSHPrivateKeyRef,
			SSHPublicKeyRef:      *status.SSHPublicKeyRef,
			ServiceRef:           *status.ServiceRef,
			AsFirstNode:          false,
		},
	}
	if _, deployment, err := k8s_etcdnodedeployment.Reconcile(
		ctx,
		c,
		newEtcdNodeDeploymentName(e),
		e.GetNamespace(),
		k8s_etcdnodedeployment.WithReplicas(replicas),
		k8s_etcdnodedeployment.WithSelector(newEtcdNodeDeploymentSelector(e)),
		k8s_etcdnodedeployment.WithTemplate(&template),
	); err != nil {
		return nil, fmt.Errorf("unable to reconcile EtcdNodeDeployment: %w", err)
	} else {
		return deployment, nil
	}
}

func finalizeEtcdNodeDeployments(
	ctx context.Context,
	c client.Client,
	e client.Object,
) error {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "finalizeEtcdNodeDeployments")
	defer span.End()

	if op, deployment, err := k8s_etcdnodedeployment.Update(
		ctx,
		c,
		newEtcdNodeDeploymentName(e),
		e.GetNamespace(),
		k8s_etcdnodedeployment.WithReplicas(0),
	); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("unable to scale EtcdNodeDeployment to zero: %w", err)
	} else if op != controllerutil.OperationResultNone {
		return errors.NewRequeueError("waiting for EtcdDeployments are finalized").WithDelay(30 * time.Second)
	} else {
		return finalizer.FinalizeObject(ctx, c, deployment.GetNamespace(), deployment.GetName(), deployment)
	}
}
