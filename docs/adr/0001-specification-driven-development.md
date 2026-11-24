# 0001. Specification-driven development with AI agents

- Status: proposed
- Date: 2026-11-24

## Context

Most of the code in this project is going to be written by AI agents, and the project has a single maintainer.
Reviewing every line an agent writes does not scale and is not a good use of the maintainer's attention. Without
line-by-line review, something else has to establish that a change does what was intended and nothing else.

## Options

1. **Review every line.** Keeps the usual guarantees, but makes the maintainer the bottleneck and gives up most of
   the benefit of agents.
2. **Trust the agent and review the result by hand.** Fast, but there is no record of what was intended, and
   regressions go unnoticed.
3. **Review specifications and evidence.** The maintainer reviews what the software must do, and machines verify
   that it does. Needs up-front investment in specifications, tests, and guardrails.

## Decision

Option 3. Human review moves from implementation code to:

- specifications in `docs/specs`, whose requirements have identifiers that tests claim,
- the traceability from requirements to tests, produced by `make spec-check`,
- changes to the API and to the operator's permissions, which are readable as manifest diffs, and
- changes to the guardrails themselves.

Everything else is gated by CI: tests, linters, generated-file checks, and the image build. Agents work under the
rules in `CLAUDE.md` and the hooks in `.claude/`.

The pull request author merges manually for now. Automatic merging of changes outside the protected paths is to be
considered once the guardrails have proven themselves.

## Consequences

- A behavior change needs a specification change first. This is slower for small changes and is accepted as the
  price of knowing what the software is supposed to do.
- The guarantee is only as strong as the tests. A test that claims a requirement without verifying it defeats the
  scheme, so tests are written from the specification before the implementation, and are never weakened to pass.
- Code that predates this decision has no specification. Its behavior is to be written down as specifications
  before it is changed.
- The guardrails are part of the trusted base. Changes to them always need a human review.
