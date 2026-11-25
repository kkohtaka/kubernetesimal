# Plan for the installation specification

This plan belongs to the [specification of installing the operator](spec.md).

## Decisions

These were decided while drafting and are the owner's to overrule before the specification is approved.

1. **Metrics are not exposed for now (INST-007).** The metrics proxy is removed, and the metrics endpoint listens on
   the loopback address of the Pod only, so nothing outside the Pod can scrape it. The version of controller-runtime
   in use cannot protect the endpoint by itself. Once the dependencies are updated, issue #287, the endpoint can be
   served with authentication, and a requirement for reading metrics with credentials can be added then.
2. **Tracing is off unless configured (INST-005, INST-006).** The address of the collector leaves the default
   configuration. The option stays.
3. **cert-manager is not needed (INST-002).** This follows from the etcd specification, which moves validation into
   the schema and removes the webhook. Until that step is done, the manifests still need cert-manager, and INST-002
   is not met.
4. **The memory limit is set from a measurement (INST-008).** The end-to-end test records how much memory the
   operator uses while it runs three members, and the limit is set to a round number well above that. The
   experiments ran with 512Mi and never measured the use.
5. **Upgrading and uninstalling are not specified.** There is no release to upgrade from yet.

## Design

All of the work is in `config/`:

- `config/default` drops the patch that adds the `kube-rbac-proxy` container, and the manifests under `config/rbac`
  that only served it: the proxy's role, role binding, client role, and Service.
- `config/manager/controller_manager_config.yaml` drops the tracing section, and binds metrics to
  `127.0.0.1:8080`, which the command line already says.
- `config/manager/manager.yaml` gets requests and limits from the measurement.
- `config/manager/kustomization.yaml` names the image on `ghcr.io`.
- The kustomization files replace the deprecated `bases`, `patchesStrategicMerge`, and `vars`.
- When the webhook is removed by the etcd plan: `config/webhook`, `config/certmanager`, and the patches and
  variables in `config/default` that connect them go too.
- `config/prometheus` stays unused, as it is now, and is removed with the proxy.

Two things are outside `config/`:

- `main.go` reads the configuration file through a package that controller-runtime has deprecated and is going to
  remove. Reading it directly, with the same fields, is small and unblocks the dependency update.
- `hack/release/manifest.sh` replaces the image by matching a pattern. With the image named on `ghcr.io` it only has
  to set the tag.

`hack/e2e` on the experiment branch is an overlay that repairs `config/default`. Once `config/default` works, the
end-to-end tests deploy it directly, with an overlay only for the image that was built in the test run.

## How each requirement is tested

| Requirement | Test | Notes |
|---|---|---|
| INST-001 | end-to-end | The test suite begins by applying the manifests. |
| INST-002 | end-to-end | The test cluster has no cert-manager. |
| INST-003 | end-to-end | Follows from INST-001 on a runner without registry credentials. |
| INST-004 | unit | Renders the manifests and reads the image. The release half is checked by `hack/release/manifest.sh` itself, which fails when it finds no image to replace. |
| INST-005 | end-to-end | Reads the operator's log after the suite and looks for export errors. |
| INST-006 | envtest or unit | Starts a collector stub in the test and counts spans. |
| INST-007 | end-to-end | Requests the metrics port from another Pod and expects no answer. |
| INST-008 | end-to-end | Reads the restart count of the container at the end of the suite. |
| INST-009 | end-to-end | Applies twice and compares. |
| INST-010 | end-to-end | Reads the operator's log for refused requests at the end of the suite. |

Most of these are assertions at the start and at the end of the end-to-end suite of the etcd specification, not
tests of their own. They need the Go test for claims, so the suite has one test function per requirement that the
workflow can call at the right moment.

## Order of work

1. **Make the manifests deployable**: remove the proxy, the tracing default, and the deprecated kustomize fields,
   set the image and a provisional memory limit. INST-001, INST-003, INST-004, INST-005, INST-007, INST-009. This
   can be done before anything in the etcd plan, and the end-to-end tests of that plan need it.
2. **Read the configuration file without the deprecated package.** INST-006.
3. **Set the memory limit from the measurement**, once three members can be created. INST-008.
4. **Remove what the webhook needed**, together with step 3 of the etcd plan. INST-002.
5. **Check the permissions** at the end of the complete end-to-end suite. INST-010.
6. Set `status: implemented`.

Each step that changes what users get comes with a change file.
