# Specifications

This project is developed specification first. A human decides *what* the software must do by reviewing a
specification, and reviews the *evidence* that it does so. Implementation code is not reviewed line by line.

## Workflow

1. **Draft.** Write `docs/specs/<name>/spec.md` from [`TEMPLATE.md`](TEMPLATE.md), and a `plan.md` next to it that
   describes the design. Open a pull request that contains only these documents.
2. **Approve.** The owner reviews the specification and the plan. Merging the pull request is the approval, so the
   pull request sets `status: approved`.
3. **Implement.** In a second pull request, write the tests first, each claiming the requirements it verifies, and
   commit them before the implementation. Then implement until they pass, and set `status: implemented`.
4. **Verify.** CI runs the tests and `make spec-check`, which fails when a requirement of an implemented
   specification has no test. The owner reviews the traceability table, and the API and permission changes.

Changing the behavior of something that is already specified starts again at step 1, by editing its specification.

## Format

A specification is `docs/specs/<name>/spec.md`. It starts with front matter that holds its status:

| Status | Meaning | Checked by `make spec-check` |
|---|---|---|
| `draft` | Being written. Not agreed yet. | Identifiers are unique. |
| `approved` | Agreed, but not or only partly implemented. | Identifiers are unique. |
| `implemented` | Fully implemented. | Every requirement is claimed by at least one test. |

### Requirements

A requirement is a list item that starts with a bold identifier:

```markdown
- **ETCD-012**: When `spec.replicas` is increased, members are added one at a time, and the next member is added
  only after the previous one is ready.
```

- An identifier is an upper-case area prefix, a hyphen, and three digits. It is unique across all specifications.
- A requirement describes behavior that can be observed from outside: through the Kubernetes API, on the virtual
  machines, or over the network. It does not describe how the code is structured.
- A requirement is testable. If it cannot be turned into a test that passes or fails, split it or make it concrete.
- An identifier is never reused. To retire a requirement, strike it through and say why, and remove the tests that
  claim it in the same pull request:

  ```markdown
  - ~~**ETCD-012**~~: Removed, superseded by ETCD-031.
  ```

### Claiming a requirement in a test

A test claims a requirement by putting the identifier in square brackets in its name:

```go
It("[ETCD-012] adds members one at a time", func() { ... })
```

```go
t.Run("[ETCD-012] adds members one at a time", func(t *testing.T) { ... })
```

One test may claim several requirements, and several tests may claim the same one. A test must actually verify what
it claims: a claim on a test that does not exercise the requirement is worse than no claim.

## Checking

```bash
make spec-check
```

prints the traceability table and fails when:

- an identifier is defined more than once,
- a test claims an identifier that no specification defines,
- a specification has no valid status, or
- a requirement of an implemented specification is not claimed by any test.

It checks that a claiming test exists. That the test passes is checked by `make test`, and focused or skipped tests
are rejected by `make lint`.
