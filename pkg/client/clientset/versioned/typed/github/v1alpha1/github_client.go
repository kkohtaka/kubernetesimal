// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/kkohtaka/kubernetesimal/pkg/apis/github/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/pkg/client/clientset/versioned/scheme"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	rest "k8s.io/client-go/rest"
)

type GithubV1alpha1Interface interface {
	RESTClient() rest.Interface
	RepositoriesGetter
}

// GithubV1alpha1Client is used to interact with features provided by the github.kkohtaka.org group.
type GithubV1alpha1Client struct {
	restClient rest.Interface
}

func (c *GithubV1alpha1Client) Repositories(namespace string) RepositoryInterface {
	return newRepositories(c, namespace)
}

// NewForConfig creates a new GithubV1alpha1Client for the given config.
func NewForConfig(c *rest.Config) (*GithubV1alpha1Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &GithubV1alpha1Client{client}, nil
}

// NewForConfigOrDie creates a new GithubV1alpha1Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *GithubV1alpha1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new GithubV1alpha1Client for the given RESTClient.
func New(c rest.Interface) *GithubV1alpha1Client {
	return &GithubV1alpha1Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1alpha1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *GithubV1alpha1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
