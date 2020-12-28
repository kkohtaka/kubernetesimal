module github.com/kkohtaka/kubernetesimal

go 1.13

require (
	github.com/go-logr/logr v0.3.0
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	k8s.io/apimachinery v0.20.1
	k8s.io/client-go v12.0.0+incompatible
	kubevirt.io/client-go v0.36.0
	sigs.k8s.io/controller-runtime v0.7.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v11.0.0+incompatible
	github.com/go-kit/kit => github.com/go-kit/kit v0.3.0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.3.1
	github.com/openshift/api => github.com/openshift/api v0.0.0-20191219222812-2987a591a72c
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20191125132246-f6563a70e19a
	github.com/operator-framework/operator-lifecycle-manager => github.com/operator-framework/operator-lifecycle-manager v0.0.0-20190128024246-5eb7ae5bdb7a
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.2.4

	k8s.io/api => k8s.io/api v0.20.1
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.1
	k8s.io/apiserver => k8s.io/apiserver v0.20.1
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.20.1
	k8s.io/client-go => k8s.io/client-go v0.20.1
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.20.1
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.20.1
	k8s.io/code-generator => k8s.io/code-generator v0.20.1
	k8s.io/component-base => k8s.io/component-base v0.20.1
	k8s.io/cri-api => k8s.io/cri-api v0.20.1
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.20.1
	k8s.io/klog => k8s.io/klog v0.4.0
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.20.1
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.20.1
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.20.1
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.20.1
	k8s.io/kubectl => k8s.io/kubectl v0.20.1
	k8s.io/kubelet => k8s.io/kubelet v0.20.1
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.20.1
	k8s.io/metrics => k8s.io/metrics v0.20.1
	k8s.io/node-api => k8s.io/node-api v0.20.1
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.20.1
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.20.1
	k8s.io/sample-controller => k8s.io/sample-controller v0.20.1

	kubevirt.io/containerized-data-importer => kubevirt.io/containerized-data-importer v1.26.1
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2
)
