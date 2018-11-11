package github

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	extclientset "github.com/kkohtaka/kubernetesimal/pkg/client/clientset/versioned"
	extscheme "github.com/kkohtaka/kubernetesimal/pkg/client/clientset/versioned/scheme"
	githubinformers "github.com/kkohtaka/kubernetesimal/pkg/client/informers/externalversions/github/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/pkg/client/listers/github/v1alpha1"
)

const controllerName = "repository-controller"

type Controller struct {
	kubeclientset      kubernetes.Interface
	extclientset       extclientset.Interface
	repositoriesLister v1alpha1.RepositoryLister
	repositoriesSynced cache.InformerSynced
	workqueue          workqueue.RateLimitingInterface
	recorder           record.EventRecorder
}

func NewController(
	kubeclientset kubernetes.Interface,
	extclientset extclientset.Interface,
	repositoryInformer githubinformers.RepositoryInformer,
) *Controller {
	utilruntime.Must(extscheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(
		&typedcorev1.EventSinkImpl{
			Interface: kubeclientset.CoreV1().Events(""),
		},
	)
	recorder := eventBroadcaster.NewRecorder(
		scheme.Scheme,
		corev1.EventSource{
			Component: controllerName,
		},
	)

	controller := &Controller{
		kubeclientset:      kubeclientset,
		extclientset:       extclientset,
		repositoriesLister: repositoryInformer.Lister(),
		repositoriesSynced: repositoryInformer.Informer().HasSynced,
		workqueue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"Repositories",
		),
		recorder: recorder,
	}

	klog.Info("Setting up event handlers")
	repositoryInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: controller.enqueueRepository,
			UpdateFunc: func(o, n interface{}) {
				controller.enqueueRepository(n)
			},
		},
	)

	return controller
}

func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	return nil
}

func (c *Controller) enqueueRepository(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}
