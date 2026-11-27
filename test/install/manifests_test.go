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

// Package install tests the manifests that install the operator, as the specification in docs/specs/install
// describes them. These tests render the manifests and read the result. What only a cluster can show is verified by
// the end-to-end tests.
package install

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

const (
	repositoryRoot = "../.."
	operatorImage  = "ghcr.io/kkohtaka/kubernetesimal/kubernetesimal"
	managerName    = "manager"
	// releaseVersion is a version that no release is going to have.
	releaseVersion = "v98.76.54"
)

// useKubectl puts the kubectl of envtest's assets first in PATH, so that the tests need nothing that 'make test'
// does not already install. Without the assets, the kubectl that is in PATH already is used.
func useKubectl(t *testing.T) {
	t.Helper()
	if assets := os.Getenv("KUBEBUILDER_ASSETS"); assets != "" {
		t.Setenv("PATH", assets+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Fatalf("kubectl is neither in KUBEBUILDER_ASSETS nor in PATH: %v", err)
	}
}

// run executes a command in the root of the repository and returns what it wrote to stdout and to stderr.
func run(t *testing.T, cmd *exec.Cmd) (string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd.Dir = repositoryRoot
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s failed: %v\n%s", strings.Join(cmd.Args, " "), err, stderr.String())
	}
	return stdout.String(), stderr.String()
}

func parse(t *testing.T, manifests string) []unstructured.Unstructured {
	t.Helper()
	var objects []unstructured.Unstructured
	decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifests), 4096)
	for {
		var object unstructured.Unstructured
		if err := decoder.Decode(&object); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("the manifests do not parse: %v", err)
		}
		if len(object.Object) > 0 {
			objects = append(objects, object)
		}
	}
	if len(objects) == 0 {
		t.Fatal("the manifests are empty")
	}
	return objects
}

// repositoryManifests renders what 'kubectl apply -k config/default' applies, and returns the objects and the
// warnings that rendering printed.
func repositoryManifests(t *testing.T) ([]unstructured.Unstructured, string) {
	t.Helper()
	useKubectl(t)
	stdout, stderr := run(t, exec.Command("kubectl", "kustomize", "config/default"))
	return parse(t, stdout), stderr
}

// releaseManifests renders the install.yaml of the version releaseVersion, the way the release workflow does.
func releaseManifests(t *testing.T) []unstructured.Unstructured {
	t.Helper()
	useKubectl(t)
	stdout, _ := run(t, exec.Command("hack/release/manifest.sh", releaseVersion))
	return parse(t, stdout)
}

func ofKind(objects []unstructured.Unstructured, kind string) []unstructured.Unstructured {
	var found []unstructured.Unstructured
	for _, object := range objects {
		if object.GetKind() == kind {
			found = append(found, object)
		}
	}
	return found
}

// deployments returns every Deployment in the manifests. The operator's is expected to be the only one.
func deployments(t *testing.T, objects []unstructured.Unstructured) []appsv1.Deployment {
	t.Helper()
	var found []appsv1.Deployment
	for _, object := range ofKind(objects, "Deployment") {
		var deployment appsv1.Deployment
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &deployment); err != nil {
			t.Fatalf("Deployment %s does not convert: %v", object.GetName(), err)
		}
		found = append(found, deployment)
	}
	return found
}

// containers returns every container, of every kind, that the manifests run.
func containers(t *testing.T, objects []unstructured.Unstructured) []corev1.Container {
	t.Helper()
	for _, kind := range []string{"Pod", "ReplicaSet", "StatefulSet", "DaemonSet", "Job", "CronJob"} {
		if found := ofKind(objects, kind); len(found) > 0 {
			t.Fatalf("the manifests contain a %s, which this test does not know how to read", kind)
		}
	}
	var found []corev1.Container
	for _, deployment := range deployments(t, objects) {
		spec := deployment.Spec.Template.Spec
		found = append(found, spec.InitContainers...)
		found = append(found, spec.Containers...)
	}
	if len(found) == 0 {
		t.Fatal("the manifests run no container")
	}
	return found
}

func manager(t *testing.T, objects []unstructured.Unstructured) corev1.Container {
	t.Helper()
	var found []corev1.Container
	for _, container := range containers(t, objects) {
		if container.Name == managerName {
			found = append(found, container)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly one container named %q, found %d", managerName, len(found))
	}
	return found[0]
}

// configuration returns the configuration file that the operator is started with, as the manifests hold it.
func configuration(t *testing.T, objects []unstructured.Unstructured) map[string]interface{} {
	t.Helper()
	for _, object := range ofKind(objects, "ConfigMap") {
		file, found, err := unstructured.NestedString(object.Object, "data", "controller_manager_config.yaml")
		if err != nil || !found {
			continue
		}
		var config map[string]interface{}
		if err := yaml.Unmarshal([]byte(file), &config); err != nil {
			t.Fatalf("the configuration file does not parse: %v", err)
		}
		return config
	}
	t.Fatal("the manifests hold no configuration file for the operator")
	return nil
}

func TestImage(t *testing.T) {
	t.Run("[INST-004] the manifests in the repository name the operator image on ghcr.io", func(t *testing.T) {
		objects, _ := repositoryManifests(t)
		image := manager(t, objects).Image
		name, _, _ := strings.Cut(image, ":")
		if name != operatorImage {
			t.Errorf("the operator runs the image %q, want the image %q with any tag", image, operatorImage)
		}
	})

	t.Run("[INST-004] the install.yaml of a release names the operator image with the tag of the release",
		func(t *testing.T) {
			image := manager(t, releaseManifests(t)).Image
			if want := operatorImage + ":" + releaseVersion; image != want {
				t.Errorf("the operator runs the image %q, want %q", image, want)
			}
		})
}

// The tests below fail when the manifests go back to what made them undeployable. They claim no requirement,
// because rendering shows less than the requirements say: INST-001, INST-003, INST-005, and INST-007 are about what
// happens in a cluster, and the end-to-end tests verify them there.
func TestRenderedManifests(t *testing.T) {
	objects, warnings := repositoryManifests(t)

	t.Run("rendering prints no warnings", func(t *testing.T) {
		if warnings != "" {
			t.Errorf("rendering config/default printed:\n%s", warnings)
		}
	})

	t.Run("the release names no image but the operator's", func(t *testing.T) {
		for _, container := range containers(t, releaseManifests(t)) {
			if want := operatorImage + ":" + releaseVersion; container.Image != want {
				t.Errorf("container %q runs the image %q, want %q", container.Name, container.Image, want)
			}
		}
	})

	t.Run("the operator is given no address of a trace collector", func(t *testing.T) {
		if tracing, found := configuration(t, objects)["tracing"]; found {
			t.Errorf("the configuration file configures tracing: %v", tracing)
		}
		for _, arg := range manager(t, objects).Args {
			if strings.HasPrefix(strings.TrimLeft(arg, "-"), "otel-collector") {
				t.Errorf("the operator is started with %q", arg)
			}
		}
	})

	t.Run("the metrics endpoint listens on the loopback address only", func(t *testing.T) {
		const want = "--metrics-bind-address=127.0.0.1:8080"
		// The flag has to be on the command line: its default listens on every address, and the default wins
		// over the configuration file.
		var found bool
		for _, arg := range manager(t, objects).Args {
			found = found || arg == want
		}
		if !found {
			t.Errorf("the operator is started with %q, want %q among them", manager(t, objects).Args, want)
		}
	})

	t.Run("nothing in the manifests serves or scrapes metrics", func(t *testing.T) {
		for _, container := range containers(t, objects) {
			if container.Name != managerName {
				t.Errorf("the manifests run the container %q next to the operator", container.Name)
			}
		}
		for _, object := range objects {
			if strings.Contains(object.GetName(), "metrics") || strings.Contains(object.GetName(), "proxy") {
				t.Errorf("the manifests contain the %s %q", object.GetKind(), object.GetName())
			}
		}
	})

	// INST-002 is about a cluster that has no cert-manager, and the end-to-end tests install into one. This shows
	// the same thing sooner, and for the release as well.
	t.Run("the manifests contain nothing that needs cert-manager, and no webhook", func(t *testing.T) {
		for name, rendered := range map[string][]unstructured.Unstructured{
			"config/default": objects,
			"the release":    releaseManifests(t),
		} {
			for _, object := range rendered {
				if strings.HasSuffix(object.GroupVersionKind().Group, "cert-manager.io") {
					t.Errorf("%s contains the %s %q of cert-manager", name, object.GetKind(), object.GetName())
				}
				for key := range object.GetAnnotations() {
					if strings.Contains(key, "cert-manager.io") {
						t.Errorf("%s: the %s %q has the annotation %q", name, object.GetKind(), object.GetName(), key)
					}
				}
				switch object.GetKind() {
				case "MutatingWebhookConfiguration", "ValidatingWebhookConfiguration":
					t.Errorf("%s contains the %s %q", name, object.GetKind(), object.GetName())
				}
				if strings.Contains(object.GetName(), "webhook") {
					t.Errorf("%s contains the %s %q", name, object.GetKind(), object.GetName())
				}
			}
			for _, crd := range ofKind(rendered, "CustomResourceDefinition") {
				if strategy, found, _ := unstructured.NestedString(crd.Object, "spec", "conversion", "strategy"); found &&
					strategy != "None" {
					t.Errorf("%s: %q is converted by a %s", name, crd.GetName(), strategy)
				}
			}
		}
		for _, deployment := range deployments(t, objects) {
			for _, volume := range deployment.Spec.Template.Spec.Volumes {
				if volume.Secret != nil {
					t.Errorf("the Deployment %q mounts the Secret %q, which nothing in the manifests creates",
						deployment.Name, volume.Secret.SecretName)
				}
			}
		}
	})

	t.Run("the operator gets more memory than the limit that kept it from starting", func(t *testing.T) {
		limits := manager(t, objects).Resources.Limits
		limit := limits.Memory()
		if limit.Value() <= 30*1024*1024 {
			t.Errorf("the memory limit of the operator is %s", limit.String())
		}
	})
}
