# Kubernetesimal

A Kubernetes operator that builds Kubernetes clusters on top of Kubernetes, using KubeVirt virtual machines. Today it
manages etcd clusters only; control plane and worker node management are planned.

## Language

Everything in this repository is written in English: code, comments, documentation, commit messages, issues, and pull
requests. This holds regardless of the language used in the conversation.

## Commands

| Command | Purpose |
|---|---|
| `make test` | Regenerate manifests and code, run `go fmt` and `go vet`, then run all tests with envtest. |
| `make lint` | Run golangci-lint and report issues introduced since `origin/main`. |
| `make lint-all` | Run golangci-lint and report every issue, including the pre-existing backlog. |
| `make manifests generate` | Regenerate CRDs, RBAC, webhook manifests, and deepcopy code. |
| `make build` | Build the manager binary into `bin/`. |

Run a single test package with:

```bash
KUBEBUILDER_ASSETS="$(bin/setup-envtest use 1.29.x --bin-dir bin -p path)" go test ./controllers/...
```

`make deploy`, `make install`, and `make docker-push` act on a real cluster or registry. Do not run them unless asked.

## Architecture

Custom resources (`api/v1alpha1`, group `kubernetesimal.kkohtaka.org`) form the same hierarchy as
Deployment / ReplicaSet / Pod:

`Etcd` → `EtcdNodeDeployment` → `EtcdNodeSet` → `EtcdNode`

- `controllers/etcd`: owns the PKI (CA, client, and peer certificates), the SSH key pair, the Service, the
  EndpointSlice, and one EtcdNodeDeployment. A separate prober checks etcd health.
- `controllers/etcdnodedeployment`, `controllers/etcdnodeset`: rolling updates and replica management, ported from the
  upstream Deployment and ReplicaSet controllers.
- `controllers/etcdnode`: creates a KubeVirt VirtualMachineInstance, a cloud-init Secret, and a peer Service, then
  provisions the etcd member over SSH with the scripts in `controllers/etcdnode/templates`.
- `k8s/*`: helpers that create or update one kind of object each. `controller/*`: finalizers, expectations, and
  requeue errors. `pki`, `ssh`, `net/http`, `observability/tracing`: supporting libraries.

Reconcilers take a copy of the spec and status, return the new status, and let the caller patch it. Return a
`RequeueError` from `controller/errors` to requeue without logging an error.

## Rules

Humans do not review implementation code line by line in this project. They review specifications, API changes, and
the evidence that the specification is met. The rules below keep that workable.

- **Tests are the evidence.** Every behavior change comes with a test that fails without it. Never delete, skip, or
  weaken a test to make it pass (`Skip`, `XIt`, `PIt`, `t.Skip`, loosened assertions). If a test is wrong, say so and
  stop.
- **No lint suppressions without a reason.** `//nolint` needs the linter name and an explanation, and is a last resort.
  Do not edit `.golangci.yml` to make an issue go away.
- **Generated files are never edited by hand.** `zz_generated.deepcopy.go`, `config/crd/bases/`, `config/rbac/role.yaml`,
  and `config/webhook/manifests.yaml` come from `make manifests generate`. Change the Go types or the `+kubebuilder`
  markers instead. CI fails when they are out of date.
- **Guardrails are changed only on request.** `.github/`, `.claude/`, `CLAUDE.md`, `CODEOWNERS`, `.golangci.yml`, and
  the lint and test targets in the `Makefile` define how changes are verified. Do not change them as a side effect of
  another task.
- **API and permission changes are called out.** Any change under `api/`, `config/crd/`, or `config/rbac/` is listed
  explicitly in the pull request description, because a human reviews those.
- **Prefer declarative validation.** Put API constraints in CRD schema markers and CEL rules rather than webhook Go code
  where possible, so that they are readable as a manifest diff.
- **Stay in scope.** Do not refactor or fix unrelated code in passing. Report it instead.

## Before finishing

Run `make test` and `make lint`, and make sure `git status` shows no unexpected changes to generated files.
