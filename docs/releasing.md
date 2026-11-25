# Releasing

A release is a version of the operator that users can install: a container image, the manifests that install it, a
Git tag, and a GitHub release with the notes of that version. [ADR 0002](adr/0002-releases.md) records why releases
look the way they do.

The changelog is built with [Changie](https://changie.dev) from one small file per change. Whoever makes a change
writes down what it means for users while they still know. Releasing only collects what was written.

## Describing a change

A pull request that changes what users get adds a change file:

```bash
make change KIND=Fixed BODY="The OTLP gRPC collector address from the configuration file is no longer ignored."
```

This writes a file to `.changes/unreleased`. Commit it with the change.

| Kind | Use it when | Raises |
|---|---|---|
| `Added` | Users can do something they could not do before. | minor |
| `Changed` | Something users rely on behaves differently. | minor |
| `Deprecated` | Something still works but is going to be removed. | minor |
| `Removed` | Something users could rely on is gone. | minor |
| `Fixed` | Something did not work as specified and now does. | patch |
| `Security` | A vulnerability is closed. | patch |

While the version is below 1.0.0, the API is unstable and incompatible changes raise the minor version. From 1.0.0
on, `Changed` and `Removed` have to raise the major version, which is a change to `.changie.yaml`.

Write the body for someone who uses the operator, not for someone who reads its code. Say what is different for
them, in one or two full sentences. If they have to do something when upgrading, say what. An incompatible change to
the API is also recorded in [`api-breaking-changes.md`](api-breaking-changes.md).

### When a change file is needed

`make change-check` fails a pull request that touches the operator's code, its API, or its deployment manifests
without adding a change file. Tests, generated code, tooling, documentation, and dependency updates need none.

Some changes to those paths change nothing for users, a refactoring for example. Do not write a change file that
says so. The owner exempts such a pull request by putting the `skip-changelog` label on it.

## Making a release

1. **Prepare.** Run the `release-prepare` workflow from the Actions tab. Leave the version at `auto` to derive it
   from the kinds of the unreleased changes, or give one, such as `v0.1.0` for the first release. The workflow
   collects the change files into `.changes/<version>.md`, updates `CHANGELOG.md`, and pushes the result to the
   branch `release/<version>`.
2. **Review.** Open the pull request with the link from the workflow summary. It shows exactly what the release
   notes are going to say. Fix the wording on the branch if needed.
3. **Publish.** Merge the pull request. The `release` workflow notices the new version on the main branch, publishes
   the image as `ghcr.io/kkohtaka/kubernetesimal/kubernetesimal:<version>` and as `latest`, and creates the tag and
   the GitHub release with `install.yaml` attached.

The same preparation works from a checkout, which is how an agent does it:

```bash
make release-prepare VERSION=auto
```

Commit the result on a branch and open a pull request.

The pull request is opened by a person or an agent rather than by the workflow, because pull requests that a
workflow opens with the repository's own token do not trigger CI.

### If publishing fails

The `release` workflow skips a version that already has a tag or a GitHub release, and publishes the image before it
creates either. Running it again after a failure is therefore safe: re-run the failed run from the Actions tab.

## Installing a release

```bash
kubectl apply -f https://github.com/kkohtaka/kubernetesimal/releases/download/<version>/install.yaml
```

The manifests need [cert-manager](https://cert-manager.io) for the webhook certificates and
[KubeVirt](https://kubevirt.io) for the virtual machines.
