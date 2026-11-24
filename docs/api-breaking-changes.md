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

None yet.
