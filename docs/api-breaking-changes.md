# Backward-incompatible API changes

`make api-check` compares the CustomResourceDefinitions with the base branch and fails on changes that can break
existing objects or clients: removed kinds, versions, and fields, changed types, fields that became required, and
tightened validation.

When such a change is intended, record it here in the pull request that makes it. Copy the description that
`make api-check` prints, in backticks, and add the reason and what users have to do. The check passes once every
change it finds is recorded.

This file is also the history of how the API broke, so entries are never removed.

## Changes

<!-- Newest first. Example:

### 2027-01-15

- `etcds.kubernetesimal.kkohtaka.org/v1alpha1: .spec.version: became required`: The default version was removed
  because ... Existing objects that omit the field have to set it before upgrading.
-->

### 2026-11-28

The [specification of etcd clusters](specs/etcd/spec.md) moves the defaults and the validation of the `Etcd` kind
from the admission webhook into the schema, and tightens the validation while doing so. See "API changes" there.

- `etcds.kubernetesimal.kkohtaka.org/v1alpha1: .spec.replicas: minimum raised from 0 to 1`: A cluster without
  members has lost its data, and deleting the `Etcd` object is the way to say that (ETCD-005). An object with zero
  replicas was accepted and reported the phase `Error`. Existing objects with `replicas: 0` have to be deleted or
  given at least one replica before upgrading.
- `etcds.kubernetesimal.kkohtaka.org/v1alpha1: .spec.version: pattern changed from (none) to ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`:
  The version has to be of the form `MAJOR.MINOR.PATCH` (ETCD-003). The webhook accepted every semantic version,
  including ones with a pre-release or build suffix such as `3.6.0-rc.1`, which no longer are. Existing objects with
  such a version have to name a release version before upgrading.
- `etcds.kubernetesimal.kkohtaka.org/v1alpha1: .spec.imagePersistentVolumeClaimRef` gets the rule
  `has(self.name) && self.name != ''`: The name of the image is required and must not be empty (ETCD-004).
  `make api-check` does not report this change, because it does not compare validation rules. The webhook already
  refused such an object when it was updated, but not when it was created, and an object that was created this way
  could never be updated. Existing objects without a name have to be deleted.

The admission webhook of the `Etcd` kind is removed with this change. An installation that still has the
`MutatingWebhookConfiguration` and the `ValidatingWebhookConfiguration` of the operator has to delete them: they
refuse every `Etcd` object once nothing answers them. No version was released with the webhook.
