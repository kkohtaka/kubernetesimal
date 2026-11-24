---
name: spec-draft
description: Draft or revise a specification and its implementation plan under docs/specs, before any code is written. Use when asked to specify, design, or plan a feature or a behavior change in this repository.
---

# Draft a specification

Produce `docs/specs/<name>/spec.md` and `docs/specs/<name>/plan.md` for the requested feature or behavior change.
Do not write or change implementation code or tests in this step.

Read `docs/specs/README.md` first. It defines the format and the rules for requirements.

## Steps

1. **Understand the request and the current behavior.** Read the code and the existing specifications that the
   change touches. If the request changes something an existing specification covers, revise that specification
   instead of writing a new one.
2. **Resolve what only the owner can decide.** Ask about scope and about choices between observable behaviors.
   Decide implementation details yourself. Put anything still undecided under "Open questions".
3. **Write `spec.md`** from `docs/specs/TEMPLATE.md` with `status: draft`.
   - Each requirement states one behavior that can be observed from outside and can pass or fail a test.
   - Cover failure and deletion, not only the successful path: what happens when a dependency is missing, when the
     object is deleted halfway, when the same change is applied twice.
   - Pick the next free number for each identifier. Run `make spec-check` to see the ones in use. Never reuse or
     renumber an identifier.
   - Spell out every API and permission change. The owner reviews those as manifest diffs later.
4. **Write `plan.md`**: the design in enough detail that the implementation holds no surprises.
   - Which packages change and how, new types and fields, and how the reconcilers are affected.
   - How each requirement is going to be tested: envtest, unit test, or end-to-end. Say so when a requirement
     cannot be verified automatically, and how it is verified instead.
   - Decisions that are expensive to reverse go into a record under `docs/adr`, linked from the specification.
   - The order of work, as steps that each leave the tests passing.
5. **Check.** Run `make spec-check`. Reread every requirement and ask whether two implementers could read it
   differently. If so, make it concrete.
6. **Hand over.** Summarize the requirements, the API and permission changes, and the open questions. When the
   owner agrees, set `status: approved` and commit the documents on their own, without code.

## Stop conditions

Stop and ask instead of guessing when the request conflicts with an approved specification, or when a requirement
cannot be made testable.
