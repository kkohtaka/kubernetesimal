# Architecture decision records

A decision record captures one significant design decision: the situation, the options, what was chosen, and what
follows from it. Specifications in [`docs/specs`](../specs) say what the software does; decision records say why it
is built the way it is.

Write one when a choice is expensive to reverse or would surprise a future reader, for example a choice between
architectures, a dependency that shapes the design, or a deliberate trade-off.

- Copy [`TEMPLATE.md`](TEMPLATE.md) to `NNNN-short-title.md`, using the next free number.
- A record is not edited after it is accepted, except for its status. To change a decision, write a new record and
  mark the old one as superseded.

| Status | Meaning |
|---|---|
| `proposed` | Under discussion. |
| `accepted` | Decided. Merging the pull request is the acceptance. |
| `superseded by NNNN` | Replaced by a later record. |
