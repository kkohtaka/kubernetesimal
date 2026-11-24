---
name: spec-implement
description: Implement an approved specification from docs/specs test-first, with every requirement claimed by a test. Use when asked to implement a specification or a plan in this repository.
---

# Implement a specification

Implement `docs/specs/<name>/spec.md` according to its `plan.md`. The owner does not review the implementation line
by line. They review the traceability from requirements to tests, so the tests are the product of this work as much
as the code is.

## Preconditions

- The specification has `status: approved` and no open questions. If it does not, stop and say so.
- Do not edit the requirements. If one turns out to be wrong, ambiguous, or impossible, stop and report it. The
  specification is changed with the `spec-draft` skill and approved again.

## Steps

1. **Read** the specification, the plan, and the code they touch.
2. **Write the tests first.** For every requirement, write at least one test that verifies it and claims it by
   putting the identifier in square brackets in the test name, for example `It("[ETCD-012] ...")`.
   - Derive each test from the text of the requirement, not from the implementation you have in mind.
   - Run the tests and confirm that each new one fails for the expected reason. A test that passes before the
     implementation exists verifies nothing.
   - Commit the tests on their own before writing any implementation.
3. **Implement** until the tests pass, following the order of work in the plan.
   - Do not change the committed tests to make them pass. If a test is wrong, say what is wrong and fix it in a
     separate commit that explains the correction.
   - Change the API only as the specification says. Regenerate with `make manifests generate`.
4. **Verify.** Run `make test`, `make lint`, and `make spec-check`, and make sure `git status` shows no unexpected
   changes to generated files.
5. **Finish.** Set `status: implemented` once every requirement is claimed by a passing test. If only part of the
   specification is implemented, leave it `approved` and list what remains.
6. **Report** with the pull request template: the traceability table from `make spec-check`, the API, permission,
   and guardrail changes in plain words, and anything that was not verified automatically.

## What counts as cheating

- Claiming a requirement on a test that does not exercise it.
- Asserting on something weaker than what the requirement says.
- Skipping, focusing, or deleting tests, or adding lint suppressions to get a green run.

If the honest result is that a requirement is not met, report that.
