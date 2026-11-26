# Roadmap

Kubernetesimal builds Kubernetes clusters on top of Kubernetes, using KubeVirt virtual machines. Today it manages
etcd clusters only. This roadmap takes it from there to a complete cluster created from a single resource.

Milestones are worked on in order. Each one ends when its exit criteria hold, not when its list of work is done.
Work follows the specification workflow in [`docs/specs`](specs/README.md), and decisions that shape more than one
milestone are recorded in [`docs/adr`](adr/README.md).

| Milestone | Goal | Status |
|---|---|---|
| [M0](#m0-revive-the-foundation) | The project builds, CI is green, and the guardrails for AI-assisted development exist. | In progress |
| [M1](#m1-pin-down-the-etcd-layer) | The existing behavior is fixed by specifications and tests. | Not started |
| [M2](#m2-harden-the-etcd-layer) | The etcd layer is a foundation a control plane can trust. | Not started |
| [M3](#m3-control-plane) | `kubectl` reaches an API server that the operator created. | Not started |
| [M4](#m4-worker-nodes) | Pods run. | Not started |
| [M5](#m5-one-cluster-resource) | One resource creates and upgrades a whole cluster. | Not started |

## M0. Revive the foundation

**Goal:** The project builds, CI is green, and the guardrails for AI-assisted development exist.

Done:

- Build fixed after the controller-runtime v0.16 bump, and `make test` working with current toolchains.
- Bazel removed. CI builds with Go, Make, and Docker.
- Guardrails: `CLAUDE.md`, hooks, linting, the specification workflow, and the traceability, coverage, API
  compatibility, and vulnerability checks. See [ADR 0001](adr/0001-specification-driven-development.md).
- The release process, built on [Changie](https://changie.dev): change files, the check that requires them, and the
  workflows that publish a version. See [ADR 0002](adr/0002-releases.md) and [Releasing](releasing.md).

Remaining:

- Update the dependencies: Kubernetes libraries, controller-runtime, KubeVirt, OpenTelemetry, Ginkgo v2, and Go
  itself. **This waits for the tests of M1**, which are the safety net for it. Until then the code has reachable
  known vulnerabilities, and the vulnerability check reports without failing the build.
  [ADR 0004](adr/0004-dependency-updates-and-vulnerability-alerts.md) says what happens to Dependabot's pull requests
  and alerts in the meantime, and which exceptions in its configuration this update removes.
- Write a README that explains what the project is and how to try it.
- Cut the first release. This waits for M1 as well: the install manifests do not work as they are, and on a stock
  cluster the `Etcd` object never becomes ready.

**Exit criteria:**

- `make vuln-check` finds nothing, and the CI job that runs it fails the build.
- Dependabot pull requests pass CI without manual work.
- A first version is released through the release process: a tag, a changelog entry, a container image with that
  tag, and install manifests that reference it.
- CI fails a pull request that changes the operator's behavior or API without a change file.

## M1. Pin down the etcd layer

**Goal:** The existing behavior is fixed by specifications and tests, so that it can be changed safely.

The controllers have no tests today, and the code predates the specification workflow. This milestone writes down
what the code does before anything changes it.

1. Write the specification of the etcd layer as it behaves today: creating a cluster, scaling, rolling updates,
   deletion, and the PKI. Where the current behavior is a defect, specify the intended behavior and note the
   difference. Experiments with the operator on a stock cluster, recorded in
   [ADR 0003](adr/0003-end-to-end-tests-on-hosted-runners.md), found three such defects already: the Service of an
   `Etcd` object points at addresses that Kubernetes does not route, the install manifests cannot be deployed, and
   the way virtual machines are attached to the network only works with some network plugins.
2. Write envtest tests for every reconciler, each claiming the requirements it verifies.
3. Write end-to-end tests for what envtest cannot show: a virtual machine boots and etcd forms a cluster. They run
   on GitHub-hosted runners, on kind with KubeVirt, as ADR 0003 decides.
4. Fix the defects that the specification exposed, such as the webhook that validates
   `imagePersistentVolumeClaimRef` on update but not on create.
5. Clear the lint backlog, now that tests protect the code, and make `make lint` judge all code.

Decide during this milestone, as an ADR, whether the control plane of M3 runs as Pods in the management cluster or
in virtual machines. The PKI design of M2 depends on it.

**Exit criteria:**

- The specification of the etcd layer is `implemented`.
- End-to-end tests run in CI for every pull request, and an etcd cluster of three members forms in them.
- `make lint-all` reports nothing.
- The remaining work of M0 is done.

## M2. Harden the etcd layer

**Goal:** The etcd layer is a foundation a control plane can trust.

- Replace etcdadm, which is no longer maintained and is downloaded into each virtual machine at run time.
- Redesign the PKI so that the CA private key does not leave the management cluster.
- Verify TLS certificates and SSH host keys. Both checks are disabled today.
- Rotate certificates before they expire.
- Keep etcd data on persistent disks, so that a member survives its virtual machine.
- Report status with the standard `metav1.Condition` type.

**Exit criteria:**

- No virtual machine holds the CA private key.
- Recreating a virtual machine does not lose its member's data.
- The linters report no security findings.
- Certificates rotate without downtime in an end-to-end test.

## M3. Control plane

**Goal:** `kubectl` reaches an API server that the operator created.

- Resources and controllers for kube-apiserver, kube-controller-manager, and kube-scheduler, on top of `Etcd`.
- PKI for the control plane: serving certificates, the front proxy, and service account keys.
- A kubeconfig for administrators, published as a Secret.

**Exit criteria:**

- An end-to-end test creates a control plane and `kubectl get namespaces` succeeds against it.
- The control plane survives the loss of one of its replicas.

## M4. Worker nodes

**Goal:** Pods run.

- Node pools of virtual machines, reusing the Deployment, Set, and Node structure of the etcd layer.
- Nodes that join the cluster by themselves.
- A network plugin, so that nodes become ready.

**Exit criteria:**

- An end-to-end test creates a node pool and a Pod on it reaches `Running`.
- Scaling a node pool up and down adds and drains nodes.

## M5. One cluster resource

**Goal:** One resource creates and upgrades a whole cluster.

- A `Cluster` resource that owns etcd, the control plane, and the node pools.
- Upgrades of the Kubernetes version, in the order the components require.
- Backup and restore of etcd.
- Promotion of the API from `v1alpha1` to `v1beta1`, with conversion.

**Exit criteria:**

- One manifest creates a working cluster in an end-to-end test.
- The same test upgrades the cluster by one minor version without losing workloads.
- A cluster is restored from a backup in an end-to-end test.

## Open decisions

| Decision | Needed by | Recorded in |
|---|---|---|
| Bridge or masquerade binding for virtual machines | Specification of M1 | The specification |
| Control plane as Pods or in virtual machines | End of M1 | An ADR |
| What replaces etcdadm | Start of M2 | An ADR |

Decided so far: how the project is developed ([ADR 0001](adr/0001-specification-driven-development.md)), what a
release is ([ADR 0002](adr/0002-releases.md)), and where end-to-end tests run
([ADR 0003](adr/0003-end-to-end-tests-on-hosted-runners.md)).
