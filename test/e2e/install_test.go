//go:build e2e

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

// Package e2e holds the end-to-end tests. They run against the cluster of the current kubeconfig, which
// hack/e2e/cluster.sh creates, and they change it: run them with 'make e2e', never against a cluster that matters.
//
// The tests of this file verify the specification in docs/specs/install. TestInstall comes first, because it
// installs the operator. The other tests read what a running operator does, and the workflow runs them after
// everything else, so that they see the whole run.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	repositoryRoot = "../.."
	// manifests is what the tests apply: config/default with the image of this test run.
	manifests = "hack/e2e"
	// repositoryManifests is what users apply from a checkout.
	repositoryManifests = "config/default"

	operatorNamespace  = "kubernetesimal"
	operatorDeployment = "kubernetesimal-controller-manager"
	operatorSelector   = "control-plane=controller-manager"

	// scratchNamespace holds what the tests create next to the operator.
	scratchNamespace = "e2e-install"
	clientImage      = "busybox:1.36"
)

// command prepares a command that runs in the root of the repository.
func command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = repositoryRoot
	return cmd
}

// output runs a command and returns what it wrote to stdout and stderr together, and whether it succeeded.
func output(cmd *exec.Cmd) (string, error) {
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// mustOutput runs a command that the test cannot go on without.
func mustOutput(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s failed: %v\n%s%s", strings.Join(cmd.Args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func kubectl(args ...string) *exec.Cmd {
	return command("kubectl", args...)
}

// ensureScratchNamespace creates the namespace for what the tests create, unless it exists.
func ensureScratchNamespace(t *testing.T) {
	t.Helper()
	if _, err := output(kubectl("get", "namespace", scratchNamespace)); err == nil {
		return
	}
	mustOutput(t, kubectl("create", "namespace", scratchNamespace))
}

// operatorLog returns everything the operator has logged, including what a container logged before a restart.
func operatorLog(t *testing.T) string {
	t.Helper()
	pods := strings.Fields(mustOutput(t, kubectl("-n", operatorNamespace, "get", "pods", "-l", operatorSelector,
		"-o", "jsonpath={.items[*].metadata.name}")))
	if len(pods) == 0 {
		t.Fatal("the operator has no Pod")
	}
	var log strings.Builder
	for _, pod := range pods {
		// The log of a previous container exists only after a restart.
		previous, _ := output(kubectl("-n", operatorNamespace, "logs", pod, "--all-containers", "--previous"))
		log.WriteString(previous)
		log.WriteString(mustOutput(t, kubectl("-n", operatorNamespace, "logs", pod, "--all-containers", "--tail=-1")))
	}
	if log.Len() == 0 {
		t.Fatal("the operator has logged nothing, so its log shows nothing")
	}
	return log.String()
}

// TestInstall installs the operator into a fresh cluster. hack/e2e/cluster.sh gives the cluster a network plugin and
// KubeVirt, and nothing else.
func TestInstall(t *testing.T) {
	t.Run("[INST-003] every image that the manifests name can be pulled without credentials", func(t *testing.T) {
		rendered := mustOutput(t, kubectl("kustomize", repositoryManifests))
		images := map[string]bool{}
		for _, match := range regexp.MustCompile(`(?m)^\s*(?:-\s+)?image:\s*["']?([^\s"']+)`).
			FindAllStringSubmatch(rendered, -1) {
			images[match[1]] = true
		}
		if len(images) == 0 {
			t.Fatal("the manifests name no image")
		}
		// An empty configuration directory holds no credentials, whatever the machine is logged in to.
		config := t.TempDir()
		for image := range images {
			if out, err := output(command("docker", "--config", config, "pull", "--quiet", image)); err != nil {
				t.Errorf("%s cannot be pulled without credentials: %v\n%s", image, err, out)
			}
		}
	})

	t.Run("[INST-001] applying the manifests succeeds and the operator becomes available within five minutes",
		func(t *testing.T) {
			start := time.Now()
			if out, err := output(kubectl("apply", "-k", manifests)); err != nil {
				t.Fatalf("applying the manifests failed: %v\n%s", err, out)
			}
			remaining := 5*time.Minute - time.Since(start)
			out, err := output(kubectl("-n", operatorNamespace, "wait", "deployment", operatorDeployment,
				"--for=condition=Available", fmt.Sprintf("--timeout=%ds", int(remaining.Seconds()))))
			if err != nil {
				t.Fatalf("the Deployment of the operator did not become available within five minutes: %v\n%s",
					err, out)
			}
		})

	t.Run("[INST-002] the operator is available in a cluster that has no cert-manager", func(t *testing.T) {
		// Whatever the manifests needed of cert-manager, they would need its kinds.
		groups := mustOutput(t, kubectl("get", "customresourcedefinitions", "-o",
			`jsonpath={range .items[*]}{.spec.group}{"\n"}{end}`))
		if strings.TrimSpace(groups) == "" {
			t.Fatal("the cluster has no CustomResourceDefinitions, not even the operator's")
		}
		for _, group := range strings.Fields(groups) {
			if group == "cert-manager.io" || strings.HasSuffix(group, ".cert-manager.io") {
				t.Errorf("the cluster has cert-manager: it serves the API group %q", group)
			}
		}
		if out, err := output(kubectl("get", "namespace", "cert-manager")); err == nil {
			t.Errorf("the cluster has cert-manager: it has the namespace\n%s", out)
		}

		// The Deployment may have become available for a moment and stopped. It has to be available now, with
		// every replica ready, and every object of the manifests has to exist.
		out, err := output(kubectl("-n", operatorNamespace, "wait", "deployment", operatorDeployment,
			"--for=condition=Available", "--timeout=60s"))
		if err != nil {
			t.Fatalf("the Deployment of the operator is not available: %v\n%s", err, out)
		}
		if out, err := output(kubectl("get", "-k", manifests, "-o", "name")); err != nil {
			t.Errorf("not every object of the manifests exists: %v\n%s", err, out)
		}
	})

	t.Run("[INST-009] applying the manifests a second time changes nothing and succeeds", func(t *testing.T) {
		snapshot := func() string {
			// The resource version of an object changes with every write to it, by anyone.
			return mustOutput(t, kubectl("get", "-k", manifests, "-o",
				`jsonpath={range .items[*]}{.kind}/{.metadata.name} {.metadata.resourceVersion}{"\n"}{end}`))
		}
		// Controllers of the cluster may still write to the objects after the first apply, as the one that fills in
		// the status of a Deployment does. Wait until the objects are at rest, so that a change can only come from
		// applying again.
		before := snapshot()
		for rest := time.Now(); time.Since(rest) < 30*time.Second; time.Sleep(5 * time.Second) {
			if now := snapshot(); now != before {
				before, rest = now, time.Now()
			}
		}

		out, err := output(kubectl("apply", "-k", manifests))
		if err != nil {
			t.Fatalf("applying the manifests a second time failed: %v\n%s", err, out)
		}
		// What kubectl prints does not tell: it reports an object as "configured" whenever it sends a patch, also
		// when the patch changes nothing. The resource versions tell whether anything was written.
		t.Logf("kubectl reported:\n%s", out)
		if after := snapshot(); after != before {
			t.Errorf("applying the manifests a second time changed objects.\nBefore:\n%s\nAfter:\n%s", before, after)
		}
	})
}

// TestMetricsAreNotExposed asks for the metrics from a Pod in another namespace, on every address and port that the
// operator and its Services have.
func TestMetricsAreNotExposed(t *testing.T) {
	t.Run("[INST-007] the metrics cannot be read from another Pod without credentials", func(t *testing.T) {
		ensureScratchNamespace(t)

		var pods struct {
			Items []struct {
				Status struct {
					PodIP string `json:"podIP"`
				} `json:"status"`
				Spec struct {
					Containers []struct {
						Ports []struct {
							ContainerPort int `json:"containerPort"`
						} `json:"ports"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"items"`
		}
		raw := mustOutput(t, kubectl("-n", operatorNamespace, "get", "pods", "-l", operatorSelector, "-o", "json"))
		if err := json.Unmarshal([]byte(raw), &pods); err != nil {
			t.Fatal(err)
		}
		if len(pods.Items) == 0 {
			t.Fatal("the operator has no Pod")
		}

		var services struct {
			Items []struct {
				Metadata struct {
					Name string `json:"name"`
				} `json:"metadata"`
				Spec struct {
					Ports []struct {
						Port int `json:"port"`
					} `json:"ports"`
				} `json:"spec"`
			} `json:"items"`
		}
		raw = mustOutput(t, kubectl("-n", operatorNamespace, "get", "services", "-o", "json"))
		if err := json.Unmarshal([]byte(raw), &services); err != nil {
			t.Fatal(err)
		}

		// Where controller-runtime serves metrics by default, where the proxy served them, and whatever the
		// Pods and the Services declare.
		var targets, healthz []string
		for _, pod := range pods.Items {
			ports := map[int]bool{8080: true, 8443: true}
			for _, container := range pod.Spec.Containers {
				for _, port := range container.Ports {
					ports[port.ContainerPort] = true
				}
			}
			for port := range ports {
				targets = append(targets, fmt.Sprintf("%s:%d", pod.Status.PodIP, port))
			}
			healthz = append(healthz, fmt.Sprintf("http://%s:8081/healthz", pod.Status.PodIP))
		}
		for _, service := range services.Items {
			for _, port := range service.Spec.Ports {
				targets = append(targets,
					fmt.Sprintf("%s.%s.svc:%d", service.Metadata.Name, operatorNamespace, port.Port))
			}
		}

		// The client prints one line per request. Reading the health endpoint shows that the client reaches the
		// operator at all: without that, not reaching the metrics would say nothing.
		var script strings.Builder
		for _, url := range healthz {
			fmt.Fprintf(&script, "wget -q -T 10 -O - %s > /dev/null && echo 'REACHED %s' || echo 'MISSED %s'\n",
				url, url, url)
		}
		for _, target := range targets {
			for _, scheme := range []string{"http", "https"} {
				url := fmt.Sprintf("%s://%s/metrics", scheme, target)
				fmt.Fprintf(&script,
					"wget -q -T 10 --no-check-certificate -O - %s > /dev/null 2>&1 && echo 'READ %s' || echo 'REFUSED %s'\n",
					url, url, url)
			}
		}

		name := fmt.Sprintf("metrics-client-%d", time.Now().Unix())
		// The log is read after the client has finished: attaching to it loses the end of what it prints.
		mustOutput(t, kubectl("-n", scratchNamespace, "run", name, "--image", clientImage,
			"--restart=Never", "--command", "--", "sh", "-c", script.String()))
		t.Cleanup(func() {
			_, _ = output(kubectl("-n", scratchNamespace, "delete", "pod", name, "--wait=false"))
		})
		mustOutput(t, kubectl("-n", scratchNamespace, "wait", "pod", name,
			"--for=jsonpath={.status.phase}=Succeeded", "--timeout=10m"))
		out := mustOutput(t, kubectl("-n", scratchNamespace, "logs", name, "--tail=-1"))
		t.Logf("the client reported:\n%s", out)

		for _, url := range healthz {
			if !strings.Contains(out, "REACHED "+url+"\n") {
				t.Errorf("the client did not reach the operator at %s, so this test shows nothing", url)
			}
		}
		if answered := strings.Count(out, "REFUSED ") + strings.Count(out, "READ "); answered != 2*len(targets) {
			t.Errorf("the client reported on %d requests for metrics, want %d", answered, 2*len(targets))
		}
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "READ ") {
				t.Errorf("the metrics were read without credentials: %s", line)
			}
		}
	})
}

// TestNoTraces reads the log of the operator for signs of sending traces. Run it at the end, so that it sees
// everything the operator did.
func TestNoTraces(t *testing.T) {
	t.Run("[INST-005] as installed, the operator logs no errors about sending traces", func(t *testing.T) {
		ensureScratchNamespace(t)

		// The operator records a trace when it reconciles. Give it something to reconcile, so that it has
		// traces to send if it is configured to send any. The object does not have to become a cluster.
		etcd := fmt.Sprintf(`{"apiVersion": "kubernetesimal.kkohtaka.org/v1alpha1", "kind": "Etcd",
			"metadata": {"name": "traces", "namespace": %q},
			"spec": {"version": "3.5.6", "replicas": 1,
				"imagePersistentVolumeClaimRef": {"name": "missing"},
				"loginPasswordSecretKeySelector": {"name": "missing", "key": "password"}}}`, scratchNamespace)
		apply := kubectl("apply", "-f", "-")
		apply.Stdin = strings.NewReader(etcd)
		mustOutput(t, apply)
		t.Cleanup(func() {
			// Whether the object goes away is not what this test is about.
			_, _ = output(kubectl("-n", scratchNamespace, "delete", "etcd", "traces", "--wait=false"))
		})

		// The status of the object shows that the operator has reconciled it.
		deadline := time.Now().Add(2 * time.Minute)
		for {
			status := mustOutput(t, kubectl("-n", scratchNamespace, "get", "etcd", "traces",
				"-o", "jsonpath={.status}{.metadata.finalizers}"))
			if strings.Trim(strings.TrimSpace(status), "{}[]") != "" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the operator did not reconcile the Etcd object, so its log shows nothing about traces")
			}
			time.Sleep(5 * time.Second)
		}
		// Traces are sent in batches every few seconds, and a failure is logged when sending times out.
		time.Sleep(time.Minute)

		problem := regexp.MustCompile(`(?i)traces? export|otlp|otel|opentelemetry|exporter`)
		for _, line := range strings.Split(operatorLog(t), "\n") {
			if problem.MatchString(line) {
				t.Errorf("the operator logged about sending traces: %s", line)
			}
		}
	})
}
