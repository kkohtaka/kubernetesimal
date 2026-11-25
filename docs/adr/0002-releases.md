# 0002. Releases built from change files

- Status: accepted
- Date: 2026-11-25

## Context

The project has never had a release. Every push to the main branch published a container image tagged `latest`,
which could point at a commit that nobody had verified, and nothing recorded what changed between two images. Users
need versions they can pin and notes that tell them what an upgrade does to them.

Most changes are written by AI agents and are not reviewed line by line, see
[ADR 0001](0001-specification-driven-development.md). What a change means for users therefore has to be stated
deliberately by whoever makes it, in a form a human can review, rather than reconstructed later from commits.

## Options

1. **Changesets.** One file per change, collected at release time. Built around npm packages: it needs a
   `package.json` and a Node.js toolchain, and it publishes to npm.
2. **release-please.** Derives versions and notes from Conventional Commits and maintains a release pull request.
   The notes are only as good as the individual commit messages, which are a by-product here: pull requests are
   merged by rebasing, so every commit an agent writes would become a line of the changelog.
3. **The Kubernetes SIG tooling.** Notes are generated from pull request titles with emoji prefixes. It covers the
   notes, and leaves the release itself to a manual process run by a release team.
4. **Changie.** One file per change like Changesets, as a single Go binary that is not tied to any language.

## Decision

Changie. A pull request that changes what users get adds a change file, and CI enforces that. Releasing collects the
files. [`docs/releasing.md`](../releasing.md) describes the process.

A release consists of:

- a Git tag `vX.Y.Z` and a GitHub release whose text is the notes of that version,
- the container image `ghcr.io/kkohtaka/kubernetesimal/kubernetesimal:vX.Y.Z`, also tagged `latest`, and
- `install.yaml`, the manifests that install that image, attached to the GitHub release.

Images are published by releases only. The main branch no longer publishes `latest`.

Versions follow Semantic Versioning and start at `v0.1.0`. Below `1.0.0`, an incompatible change raises the minor
version. The version of the operator is independent of the API version: `v1alpha1` says how stable the API is, not
which release it shipped in.

A release takes two human actions: starting the workflow that prepares it, and merging the pull request that
contains its notes. There is no standing release pull request, because releases are infrequent for now.

## Consequences

- Writing the change file is part of making a change. Agents do it as a step of implementing a specification.
- A change to the operator's code that changes nothing for users needs a human to label the pull request, which is
  deliberate friction: the claim "users will not notice" is exactly the kind that should not be made unreviewed.
- There is no Helm chart. One can be added to a release later without changing the process.
- Changie is maintained by one person. It only turns files into Markdown, so replacing it would be cheap.
- Nothing stops a release of an operator that does not work. Until end-to-end tests exist, releasing is a judgment
  call for the owner.
