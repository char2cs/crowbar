# A locked screen blocks EVERY native capture — needs the user

**Raised 2026-08-02. Needs a human to unlock the machine. Nothing else clears it.**

This is the same environment as
`hover-and-focus-need-an-unlocked-screen.md` (resolved once, recurred), but the
symptom is far broader. That one cost two flags. **This one costs every capture
on both sides.**

## The symptom

`crowbar-app --features driver` with `CROWBAR_ROW_SNAPSHOT` set starts, logs
`font: CalSansUI loaded`, and then **never emits and never exits**. It sits in
`-[NSApplication run]` (confirmed with `sample`). A freshly launched gpui app on
a locked machine never activates, never draws, and therefore never reaches the
settled frame that P3.17's capture stops on.

## The evidence, measured twice independently

| probe | result |
|---|---|
| `--surface popover --viewport-width 1714` | hung; killed at 120s |
| `--width 294 --content short --added 1 --deleted 0 --no-directory` — the **pinned Phase 1 cell**, captured fine ~1h earlier | hung; still nothing at 40s |
| same cell on the **pristine** `verify-p317` binary (merged code, untouched) | hangs identically |
| `lsappinfo front` | `loginwindow` |
| `pmset -g assertions` → `UserIsActive` | **0** before any intervention |

A worker on P3.19 reached the same diagnosis from the other end and noted **two
other sessions wedged the same way at the same time**.

**It is not a regression.** The control is the pristine binary on a cell nobody
touched, and it hangs the same way.

> ## ‼️ AMENDED 2026-08-03 — the lock was real, and the last line above was wrong
>
> **The 2026-08-02 diagnosis stands on its own evidence.** `lsappinfo front`
> returning `loginwindow` and `UserIsActive` at 0 are direct observations of a
> locked session, and no code defect produces those. That day, the screen was
> locked.
>
> **But "it is not a regression" does not follow from the control I used, and it
> is now known to be false.** The "pristine `verify-p317` binary" was built from
> the *merged code under test*. It shares every line of the capture path with the
> binary it was controlling for, so it could only ever have detected an
> uncommitted local edit — not a regression. **A control that contains the
> suspect is not a control.**
>
> Re-run on 2026-08-03 with a genuinely independent binary, on an unlocked
> machine (`CGSSessionScreenIsLocked` absent from `IOConsoleUsers`):
>
> | binary | AeroSpace on | AeroSpace off |
> |---|---|---|
> | built ~07-31, before P3.17 (`scratchpad/control-jul31-crowbar-app`) | **exit 0, snapshot** | **exit 0, snapshot** |
> | `rewrite/rust` @ `5da59b8f` | **HUNG** | **HUNG** |
>
> 24 of 24 hangs across three surfaces, including the Phase 1 canary. **There is
> a regression, it is deterministic, and it produces this file's exact symptom on
> an unlocked machine.** It is being bisected as P3.41; see QUEUE.md.
>
> ### What this changes about using this file
>
> The symptom described above — starts, logs `font: CalSansUI loaded`, never
> emits, never exits, sits in `-[NSApplication run]` — is now known to have **two
> causes**, and they are indistinguishable from the outside. **Do not reach for
> this file on the strength of the symptom alone.** Establish which one you have:
>
> ```bash
> # 1. Is the screen actually locked? The key exists ONLY when it is.
> python3 -c "import subprocess,plistlib; \
>   print([k for u in plistlib.loads(subprocess.run(['ioreg','-n','Root','-d1','-a'],\
>   capture_output=True).stdout)['IOConsoleUsers'] for k in u if 'Lock' in k])"
>
> # 2. Does a binary built BEFORE the suspect code still capture?
> CROWBAR_ROW_SNAPSHOT=/tmp/probe.json <control-binary> \
>   --width 294 --content short --added 1 --deleted 0 --no-directory
> ```
>
> Empty list plus a control that captures means the tree is at fault and this
> file does not apply. Keep a known-good binary around for exactly this; the one
> named above cost nothing to preserve and settled the question in two minutes.
>
> Everything below — what does not fix a lock, what remains possible while
> blocked — is unaffected and still correct **for the locked case**.

## What does NOT fix it — do not re-derive these

- **`caffeinate -u -t 600`** asserts user activity (`UserIsActive` flips to 1)
  and the capture **still hangs**. It prevents sleep; it does not unlock.
- `setPosition` via the Tauri bridge — refused by the capability set.
- `osascript` — no assistive access.
- Waiting. There is no timeout that helps: the app is not slow, it is not
  drawing.

## What is still possible while blocked

Everything that does not need a window: `cargo build`, `clippy`, the full test
suite (**1224 passing**), `check-invariants.sh`, and — importantly — the
`row_layout` geometry assertions, which run a **real taffy layout** in-process
under `#[gpui::test]` and need no platform frame loop.

So components can still be **built and largely verified**. What cannot happen is
the step that actually matters: **taking a convergence verdict.** No native
snapshot and no live reference can be produced, so no surface can be declared
converged while this holds.

## What clears it

**A human unlocks the machine.** Then re-run the Phase 1 canaries first — if
those two come back byte-identical, the capture path is healthy and the queued
work can proceed.

## Consequence, recorded honestly

`popover` remains verified on **exactly one cell**
(`1714 · dark · normal · no flags`). The theme and width axes are **unverified**,
not "assumed fine" — and the attempt to close the theme axis is what surfaced
both this blocker and a fabricated reference (see QUEUE.md, P3.23).
