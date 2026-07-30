# Oracle corpus — APPEND-ONLY

Action sequences the orchestrator drives both apps through. Spec §8.4.

**Rules, absolute:**

- **Append-only.** A sequence is never rewritten and never deleted.
- **Worker agents may not write here.** Enforced by the worker contract.
  Only the orchestrator appends.
- **Every append is a git-visible admission that a defect escaped the
  orchestrator's comparison.** The commit message is prefixed `corpus:` and
  says plainly what escaped and why.

This exists because the orchestrator choosing what to compare is the one
self-grading risk the worker/orchestrator split does not remove. Making the
corpus a ratchet converts that risk from invisible to auditable.
