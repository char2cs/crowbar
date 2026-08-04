# Blocked items

One file per item killed after 3 failed convergence attempts (spec §11.5).

Each file states what was tried, and what the residual delta was — in numbers,
not adjectives. A blocked item never halts the run.

Anything requiring a product decision waits for the user rather than being
invented. In particular, **adding to the §13 accepted-deltas list is always a
user decision.**

## Status index — kept here because a stale TITLE has misled a reader before

A file whose H1 still begins "Blocked" while its body records a fix is exactly
how `hover-and-focus-need-an-unlocked-screen.md` was misread once: the
resolution sat 230 lines below a title that still said blocked, and eight lines
of reading produced the wrong conclusion. **When an item resolves, change its
title, not only its body.**

| item | state |
|---|---|
| `cla-policy.md` | ⏳ **user decision** — CLA requirement after the AGPL-only relicense |
| `vendored-crates-without-a-licence.md` | ⏳ **user decision** — two vendored crates declare no licence |
| `route-audit-red-at-head.md` | ⏳ **user decision** — `api/` is out of this port's scope per §0 |
| `resizable-needs-a-taller-display.md` | ⏳ **user decision** *(or a bigger display)* |
| `four-ported-surfaces-are-dead.md` | ⏳ **user decision** — keep or delete 5 surfaces measuring unreachable code |
| `locked-screen-blocks-every-capture.md` | ⛔ **user action** — only unlocking the machine clears it; currently stranding 7 unverified surfaces |
| `four-verdicts-needed-a-repo.md` | ✅ resolved — was an IndexedDB `VersionError`, not a missing repo |
| `hover-and-focus-need-an-unlocked-screen.md` | ✅ resolved 2026-07-31 |
| `repo-import-dialog-duplicate-button-id.md` | ✅ resolved by P3.74 |
| `s13-native-menus-accepted-delta.md` | ✅ decided by the user |

**Six open, and every one of them is the user's** — five product decisions and
one physical action. That is §17.7's condition met, though by a less
comfortable route than "nothing is blocked".
