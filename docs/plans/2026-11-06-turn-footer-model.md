# A turn's footer names the model it really ran on

## Problem

The turn footer reads the model from the ledger: `runner_started.launchModel`
first, then any later `model_changed` marker (web fold `facts.model`, recorded
per turn in `facts.models`). When a chat launches with no model chosen, the
CLI picks its own default. `launchModel` is then `""` and the footer shows no
model, though the CLI did name one:

- claude's SessionStart hook carries `model`. Ingress passes it to
  `reportLaunch`, which stores it as the live runner's `ReportedModel` for the
  picker (`dto.LaunchModel = cmp.Or(ReportedModel, LaunchModel)`). It never
  reaches the ledger.
- codex's `thread/start` and `thread/resume` responses carry a top-level
  `model` (seen live on 0.167). The descriptor does not capture it, so codex
  never reports a model at all.

## Why this is a plan, not a patch

The footer must survive reload and restart, so it reads the ledger. The
reported model lives only on the live runner. Copying it into a ledger row is
a second copy of the same fact. That needs a decision on which one owns it.

## Proposal

1. The ledger owns "the model this runner runs". `ReportedModel` stops being a
   separate live field: `reportLaunch` appends a fact-only row, and the DTO and
   picker read it from the ledger's current state, as they already do for
   `launchModel`. The row is a `runner_reported` marker kind (not in
   PILL_MARKERS, so it draws nothing) or a `reportedModel` field on a
   follow-up `runner_started`. Write it only when the reported model differs
   from what the ledger already says.
2. codex: `capture: { model: model }` on `thread/start` and `thread/resume`.
   apiconn passes the captured value to `reportLaunch`, as the hooks
   SessionStart path does.
3. Web: `applyMarker` sets `facts.model` from the new marker, as it does for
   `model_changed`; `applyTurnStarted` already records it per turn.

## Tests

- Go: a default-launched runner whose CLI reports `m` appends exactly one fact
  row, and none when it reports the launch model again.
- Descriptor: the codex resume response fixture captures `model`.
- Web fold: after the fact row, `turn_started` records `models[turn] = m`.
