# Port Queue

Source of truth for the Rust-native GPUI port. Spec:
`docs/superpowers/specs/2026-07-30-rust-native-desktop-port-design.md`.
Updated every orchestrator iteration. This file is how a cold session picks up.

**Phase:** 1 — the driver and the oracle. **THE GATE.**
**Line coverage (logic crates):** `oracle` **100.00%** (2817/2817) · `crowbar-driver` **100.00%** (1134/1134) · `crowbar-core` **100.00%** (148/148) · `crowbar-client` **99.64%**. `proto`/`diff` still empty. **191 tests, 0 failed.** All measured by me.
**Corpus coverage (view crates):** the gate row **PASSES on all 18 runnable cells** (3 widths × 2 themes × 3 content lengths). The 6 state flags are **vacuous on this component** — no live consumer sets them — so a stateful target is needed to close that axis.

---

## Environment facts (verified 2026-07-30)

Recorded because a cold session will otherwise rediscover them the hard way.

| Fact | Value |
|---|---|
| `rustc` / `cargo` | **1.96.0**, installed but **NOT on the default PATH**. Every shell needs `export PATH="$HOME/.cargo/bin:$PATH"`. |
| rustup toolchains | `stable` (active), `1.85`, `1.88` |
| Installed targets | `aarch64-apple-darwin`, `x86_64-apple-darwin`, `x86_64-pc-windows-msvc`, `x86_64-unknown-linux-gnu` |
| `cargo-llvm-cov` | installed 2026-07-30 (the §12 gate tool). |
| `cargo-nextest` | not installed |
| Go | 1.26.2 (`/opt/homebrew/bin/go`) |
| `bun` | `~/.bun/bin/bun`, also off the default PATH |
| Shell is **zsh** | **Unquoted `$var` does NOT word-split**, unlike bash. Building an args string and passing `$args` sends it as ONE argument. Use an array or `${=args}`. Cost me a silently-empty sweep that looked like the binary was broken. |
| `timeout(1)` | **not installed** on macOS. `gtimeout` needs coreutils. A command using it fails with `command not found` and the surrounding pipeline still reports success. |
| Zed | `/Applications/Zed.app` present (stable channel) — used by the §10.4 AX spike |
| Network | reachable |
| `go build` of a **main** package | fails with `error obtaining VCS status: exit status 128` — Go's buildvcs stamping walks up and finds the parent repo's working tree. **Always pass `-buildvcs=false`.** Pre-existing and environmental; reproduces on a pristine checkout at this path. |
| `go build ./...` untagged | fails at `cmd/crowbar/web_embed.go`: `pattern all:web/dist: no matching files found`. Needs `make embed-web` first, or the repo's canonical `-tags noEmbed`. Also pre-existing. |
| Vendored gpui build cost | ~455 crates, **6m41s** cold release, **1.2 GB** of `target/` (now **5.5 GB** with debug + coverage profiles). Budget for it. |
| **⚠ Worker worktrees fill the disk** | Each worker gets its **own** `native/target`, and a full vendored-gpui build is **8–13 GB**. Ten workers took the machine to **121 MiB free** and P1.10 died mid-build with `No space left on device`. At zero bytes **even Bash stops working** — the harness ENOSPCs creating its own output file *before* the command runs, and `Write`/`Edit` fail on their temp files, so you cannot dig yourself out. **Clean each worker's `native/target` at merge time.** Recovering 8 merged workers' caches freed **66 GB**. |
| **Never run two cargo invocations at once** | They deadlock on `~/.cargo/.package-cache`. Symptom is indistinguishable from a slow build: no `rustc` processes, `target/` mtime frozen, and cargo prints **nothing at all** — not even "Blocking waiting for file lock". I lost ~10 minutes to this by launching a second gate run before the first finished. Check with `pgrep -c rustc`; zero rustc plus a stale `target/` mtime means blocked, not building. |
| Killed cargo runs | A killed gate script can leave an **adversarial test edit in the source tree** if it dies before its revert. Always `git status` after killing one. |
| Running daemon | a `crowbar-api` from the **`feature/crowbar-skill` worktree** is live on a temp socket. It is **not** this port's daemon; do not reuse it for parity runs. |

### The shared `CROWBAR_HOME` for parity runs (item 0.4)

`Makefile:11` scopes every `dev*` target to `CROWBAR_HOME ?= $(CURDIR)/.crowbar`.
Both apps inherit it, so "one daemon, shared home" is already the harness — the
native app only has to derive the same socket path.

| | |
|---|---|
| `CROWBAR_HOME` | `<worktree>/.crowbar` |
| fnv1a64 of it | `6d4f21ce150add3c` |
| Socket | `$TMPDIR/crowbar-6d4f21ce150add3c.sock` |
| Present? | **yes** — daemon live, pid 62909, built from this worktree |

### 0.4 harness — one daemon, both apps. **No code required.**

Worked out by reading `desktop/src-tauri/src/sidecar/mod.rs`. The rule:

> **The Tauri app owns the daemon. The native app is just another unix-socket
> client on the same path.** Start Crowbar-React first and leave it open.

That works because the socket path is a *pure function of `CROWBAR_HOME`*, both
apps set the same `CROWBAR_HOME` (`Makefile:11`, `desktop/Makefile:12`), and
`crowbar-client` connects directly to the socket with no proxy (§9.1).

> **Do NOT start a daemon manually first and then launch the Tauri app.** It
> looks like the tidier arrangement and it is a trap. `spawn()` has no
> attach-to-existing path — it unconditionally launches
> `crowbar-api serve --host unix://<path>`. Its comment at
> `sidecar/mod.rs:194` is explicit that the daemon "refuses to clobber one with
> a live daemon still behind it", so the app's child dies on bind — but
> `wait_for_health` then probes the socket and finds *your* daemon healthy, so
> the app reports success while its supervisor sits in a budgeted respawn loop
> against a socket it can never take. Symptom: everything works, with a daemon
> respawn storm in the log.

Two consequences worth knowing:

- Closing the Tauri window kills the daemon (there is a window-close kill path),
  which drops the native app's connection. Expected; not a native-app bug.
- `desktop/scripts/fetch-sidecar.sh` **builds the Go daemon from this worktree's
  source** despite its name. So daemon changes (0.6, 0.7) reach the harness on
  the next `make dev-desktop`, with no separate step.

### 0.4 — both apps against one daemon ✅ **CLOSED. Verified by my own run.**

At 2026-07-30 22:42 all three of these were alive **simultaneously**:

| | pid | |
|---|---|---|
| native `crowbar-app` | 12057 | GPUI window, layer 0, 855×1119 |
| React `crowbar-desktop` | 55007 | GPUI/WebKit window, layer 0, 855×1119 |
| daemon `crowbar-api` | 62909 | on `$TMPDIR/crowbar-6d4f21ce150add3c.sock` |

**I did not accept the worker's run as evidence.** It reported reaching the
daemon at 22:31:38; I killed that instance, baselined the daemon's health-request
count at 5, launched `crowbar-app` myself with the shared `CROWBAR_HOME`, and
watched the count go to 6 with a new line:

```
[err] 2026/07/30 22:42:00 GET /v0/health 200 783.167µs
```

Window presence confirmed via `CGWindowListCopyWindowInfo` (a Swift script —
`screencapture` and the AX API are both permission-blocked in this shell, and
`AXIsProcessTrusted()` is `false`, so pixels cannot be read here).

**The native window renders:** `daemon reached` / `pid 62909` / `status ok` /
`version 0.1.0` / the resolved socket path. Those exact strings are pinned by a
unit test, because the frame itself cannot be read back.

Gates, all re-run by me: `cargo build --workspace` clean · `cargo clippy
--workspace --all-targets -- -D warnings` **exit 0** · `cargo test --workspace`
**21 passed, 0 failed** · `check-invariants.sh` all four rules ok.

**§12 coverage, measured by me:**

```
crowbar-client   Lines 99.64%   Regions 98.23%   Functions 100.00%
  socket.rs      100.00% lines
  health.rs       99.12% lines
  lib.rs         100.00% lines
```

Above the ≥98% bar. The one uncovered line is an error arm in the test harness.

#### The zero-pad trap — closed, and the test is real

`socket.rs` formats `format!("crowbar-{hash:x}.sock")`, never `{:016x}`.
`socket_name_is_not_zero_padded` pins `/home/10/.crowbar`, and **I recomputed
the hash independently**: `fnv1a64` = `0x0bbb_5af7_c356_03fa`, which Go's `%x`
renders as `crowbar-bbb5af7c35603fa.sock` — **15** hex digits. The test asserts
the value, the short name, that the padded name is *not* produced, and that the
hex run is 15 chars. It genuinely fails under `{:016x}`.

A detail the worker added that I had not specified, and which matters: the hash
input is the `CROWBAR_HOME` string **exactly as it appears in the environment**
— not canonicalised, not trailing-slash-normalised. Go hashes the raw string, so
anything else silently disagrees.

#### Structural decisions to know about

- **Path derivation went in `crowbar-client`, not `crowbar-core`.** §4.2 gives
  `core → client`, so the edge cannot be reversed; putting it in `core` would
  mean the crate whose entire job is dialling the socket has to be *told* where
  the socket is. It is still a pure function under the same ≥98% gate.
- **`gpui` is a direct dep of `crowbar-ui`, `crowbar-state` and `crowbar-app`
  only** — not all eight permitted crates. `crowbar-ui` re-exports it
  (`pub use gpui; pub use gpui_component;`) and the leaf view crates take it
  transitively, which is what their scaffold manifests already claimed. See the
  0.1 note above for the anti-hardcoding consequence this creates.
- `crowbar-app` also needs `gpui_platform` — it owns `application()`, and
  `gpui::Application::new()` does not exist at this rev (only `with_platform`).
- **`reqwest` 0.13 has native unix-socket support** (`ClientBuilder::unix_socket`),
  so §10.1's "unix connector" needed no third-party crate. `tungstenite`
  deliberately not added until a caller exists.

> **A warning line that is not ours and cannot be removed.** Every cargo
> invocation in `native/` prints:
> `warning: the following packages contain code that will be rejected by a
> future version of Rust: block v0.1.6`.
> It is a cargo **future-incompat report**, not a rustc warning against our
> code — `block 0.1.6` is a crates.io transitive dep of the vendored gpui macOS
> stack (`static of uninhabited type`). Removing it needs either a `[patch]`
> (PINNED.md forbids inventing one) or an edit under `vendor/` (breaks the pin).
> **`clippy -D warnings` is genuinely clean.** Do not treat this line as a dirty
> gate, and do not silence it with a `.cargo/config.toml`.

> **Driving the app: the MCP bridge is on port 9224, not 9223.** The log line is
> `MCP Bridge plugin initialized for 'Crowbar' (software.rabbyte.crowbar) on
> 0.0.0.0:9224`. `driver_session` defaults to **9223**, and on this machine 9223
> is a *different* Crowbar instance from another worktree — it connects happily
> and every `execute_js` then times out after 7s against the wrong window.
> Always pass `port: 9224` (and read the real port out of the log first). Also
> note the bridge binds `0.0.0.0`, not loopback — dev-only, but the opposite of
> what 0.7 is required to do.

### Client-side persistence defeats `CROWBAR_HOME` isolation — hard evidence for D6

Chasing a 404 loop in the reference app turned up something worth more than the
bug. The app, against a **completely fresh** `CROWBAR_HOME`, was polling
`/v0/projects/835f0a4b-…/home` every 30s. The daemon has **zero** projects
(`GET /v0/projects` → `[]`) and that id appears nowhere in its `view.db`.

Where it came from, confirmed by reading it out of the running webview:

```
localStorage['crowbar.activeProject']
  = {"state":{"activeProjectId":"835f0a4b-9618-4bea-b4b8-d4468939840f"},"version":0}
```

A zustand-persist payload — **from the user's production Crowbar.**

`/Applications/Crowbar.app` and the dev build share the bundle identifier
`software.rabbyte.crowbar`, and WKWebView keys its storage container on bundle
id. So `~/Library/WebKit/software.rabbyte.crowbar/` is **one container shared by
dev and production**, and I confirmed the dev run wrote into it (files under it
are newer than the launch). The `Makefile` carefully isolates every byte of
*daemon* state per worktree, and then the webview reaches straight around that
isolation into production's client state.

**Three consequences:**

1. **This is the strongest argument for D6 in the codebase.** A stale persisted
   `activeProjectId` is never reconciled against the daemon's real project list,
   so the app chases a project that does not exist, forever, silently. The
   native client has no client-side persistence and cannot do this.
2. **§9.3's migration list is incomplete.** It names 7 read-through caches and 4
   IndexedDB stores. It does not mention **`localStorage` at all** — which holds
   **130 keys** here: the whole `crowbar:settings:*` surface (real user
   settings: fonts, theme, terminal, editor), `crowbar.activeProject`,
   `crowbar.bootstrap.appearance.v2`, `crowbar:cache-version`, and a long tail
   of `crowbar:terminal-reconnect:<uuid>` entries that appear never to be
   reaped. These need the same treatment as the four IndexedDB stores — decide
   per key: daemon-side via `/v0/settings/ui`, or deleted.
3. **Do NOT clear this storage to make the dev app tidy.** It is the user's
   production state. The 404 loop is cosmetic (the app renders OOBE correctly);
   wiping the container to silence it would destroy real settings.

The app itself is **fine**: `location.href` is `#/oobe`, rendering "The IDE where
agents do the heavy lifting", which is exactly right for a home with no projects.

#### The full `localStorage` surface, measured from the running app

**166 keys, 32,762 bytes.** Read live out of the webview. §9.3 accounts for
none of it. Classification, and what each needs:

| Group | Count | Disposition |
|---|---|---|
| `crowbar:settings:*` | **88** | **Daemon-side, `global` scope.** This is the *entire* settings surface. |
| `crowbar:terminal-reconnect:<workspaceId>` | **64** | **Delete.** See below. |
| `react-scan-*` | 3 | Delete — dev tooling. |
| `crowbar.activeProject` | 1 | **Delete.** This is the one that caused the 404 loop. |
| `crowbar.bootstrap.appearance.v2` | 1 | Daemon-side, `global` — it exists to paint before first frame. |
| `crowbar:agent-chat-order:<uuid>` | 2 | Daemon-side, per workspace. |
| `sidebar-open`, `sidebar-width` | 2 | Daemon-side, `global`. |
| `terminal-profiles` | 1 | **Check for a duplicate source of truth** — see below. |
| `crowbar:switch-profiles`, `crowbar:cache-version`, `crowbar_font_cache` | 3 | Delete — caches and a schema version that D6 makes meaningless. |
| `setItem` | 1 | Debris, see below. |

**Five things this turned up:**

1. **88 `crowbar:settings:*` keys.** Theme, fonts, editor, terminal, git, file
   tree, keybinding presets *and* user overrides, enterprise policy
   (`enterpriseManagedMode`, `enterpriseRequireExtensionAllowlist`,
   `enterpriseAllowedExtensionIds`), telemetry. §9.3 moves "four stores"
   daemon-side and never mentions this. **It is the single largest omission in
   the persistence plan** and the native client needs every one of these keys or
   users lose their entire configuration on migration. At 32 KB total the
   migration is cheap — the risk is forgetting it, not doing it.

2. **`crowbar:terminal-reconnect:*` grows without bound.** 64 entries, one per
   workspace ever opened, keyed `crowbar:terminal-reconnect:${workspaceId}`
   (`features/terminal/lib/terminal-reconnect-map.ts:4`). Each holds a
   tab-id → session-id map. Nothing reaps them when a workspace is deleted.
   D6 deletes the whole category rather than porting the leak.

3. **`terminal-profiles` may be a duplicate source of truth.** It is a
   zustand-persist blob (`{"state":{"profiles":[],...}}`) sitting in
   `localStorage`, while the daemon *already* owns
   `/v0/settings/terminal/profiles` with full CRUD
   (`endpoints/terminal/routes.go:40-44`). Worth resolving before the native
   client reads either — two writable copies of the same list is how they drift.

4. **Two un-namespaced keys.** `sidebar-open` and `sidebar-width` carry no
   `crowbar:` prefix and are written straight from a component
   (`components/layout/use-sidebar-panel.ts:61,120`). In a shared origin that is
   a collision waiting to happen, and it sidesteps the store layer that
   `CLAUDE.md` says owns persistence.

5. **A key literally named `setItem`**, whose value is
   `(k, v) => { if (k === 'sidebar-width') window.__drag.writes++; return origSet(k, v); }`
   — leftover instrumentation from a past drag-performance investigation.
   **Not a repo defect**; grep finds no such code. It persists because
   `localStorage.setItem = fn` on a `Storage` object goes through the named
   property setter and is *stored as data* instead of shadowing the method. A
   debugging session wrote a permanent key into the user's production storage
   and nobody noticed. Left in place — see consequence 3 above.

### Reference side now declares `content_sized` — verified live, 2026-07-31

Confirmed in the running app after P1.8 merged, not from the diff:
`data-oracle-content-sized` appears on **`git-row-badge` (6)**,
**`git-row-added` (4)**, **`git-row-deleted` (1)** — and **not** on
`git-row-name`, which is correctly the flexible sibling that absorbs the excess.
The extractor emits `"content_sized": true` on exactly those anchors.

Archived as `native/oracle/runs/ref-v3-content-sized.json`.

> **The v3 pair is deliberately NOT run yet.** I labelled its cell
> `state.width: 1100` — the **viewport** width, per the v1.6 ruling — while the
> native binary still labels its cell by the **surface** width (294). Running
> them now would be running with exactly the broken label I just diagnosed:
> either the differ refuses on a mismatched matrix cell, or I fudge one side's
> label to make it agree, which is the fake convergence §8.3 exists to prevent.
> **It waits for P1.9's `--viewport-width`.**

> **The `pkill -f vite` hazard fired again**, and the pre-flight check earned its
> place: my Vite on 5273 was dead — killed by another session starting a Crowbar
> — while **my app kept running against nothing**. There are now four
> `crowbar-desktop` processes on this machine, three of them other sessions'.
> Had I skipped the check I would have extracted from a stale page and compared
> it in good faith. **Run the check every time; it has now caught this twice.**

## ✅ THE STATE AXIS IS CLOSED — `selected` converges, and it found a real defect

`file-tree-row` is the second gate surface, added because the git row's state
axis was vacuous. It works.

**Driven on both sides, for real:**

| | reference (WKWebView) | native (GPUI) |
|---|---|---|
| resting `file-row-item.bg` | `#00000000` | `#00000000` |
| **`selected`** | **`#ffffff0a`** | **`#ffffff0a`** |
| `hover` | `--file-tree-hover-bg` | `#ffffff07` |
| `focus` `button.border` | `:focus-visible` rule exists | `#ffffff0d` |

The reference `selected` state was reached **programmatically** — no real pointer
input — by focusing `.file-tree-container` and dispatching a bubbling click.
Confirmed live: `data-active="true"` and `::before` painting
`oklch(1 0 0 / 0.04)`, which is exactly `#ffffff0a`.

> **Ordering matters and cost several attempts.** `highlightedPath =
> hasTreeFocus ? focusedPath : activePath`, and an effect resets `focusedPath`
> when the tree is unfocused. So focusing the container and clicking **in the
> same tick does not work** — React has not committed `hasTreeFocus` yet. Focus
> in one call, click in the next. Also: clicking a folder **toggles** it, so
> repeated calls oscillate it open/closed.

### The run found one genuine defect

```
oracle: FAIL — 1 delta over 6 anchors compared (1 colour)
file-row-name.fg: #f5f5f5ff, expected #fe9a00ff (Δ b +245, rgb is exact)
```

**The reference colours the filename by git status** — `a.ts` is modified, so it
renders amber `#fe9a00ff`. The native row paints default foreground. That is a
**missing feature in the native component**, found by the oracle rather than by
reading code, which is exactly what it is for.

> **Two of the three original deltas were MY driving error, not defects.**
> `file-row-guide-1` differed in `y` and `h` because I left `--prev-depth` /
> `--next-depth` at their defaults, so no guide capping was applied. The
> reference's `a.ts` follows `a` at depth 1, so level 1 *is* top-inset by 4px.
> Driving with `--prev-depth 1 --next-depth 2` cleared both. **The guide-inset
> logic is correct on both sides** — I had simply not told the native side who
> its neighbours were.

### Three measurements that would each have manufactured a delta on this surface

Found by the worker before compiling, and each is the same class of trap:

1. **Indent step is 16**, not the sidebar tree's 14 — `settings.fileTreeIndentSize` defaults to 16.
2. **The line box is 20px**, not the git row's 18.9 — `GitFileItem` authors `leading-[1.35]`; this row authors nothing and inherits `.text-sm`'s `calc(1.25 / 0.875)`.
3. **The icon starts at `1 + padding`** — the container-scoped `border: 1px solid transparent !important` shifts the content box. The git row's button has no border, so the two surfaces genuinely differ.

And the `file-row-button` declares **neither** `content_sized` nor `line_sized`:
it paints text and holds one line, but `h-6` authors its box at 24 around a 20px
line — the same trap as the badge.

### States this surface genuinely has

`hover` ✓ · `selected` ✓ (converges) · `focus` — rendering is real *here*
(`:focus-visible` is scoped to `.file-tree-container`, which this row is inside
and the git row is not), but **drivability unconfirmed**: the tree keeps DOM
focus on the container and moves a virtual cursor via `aria-activedescendant`,
so a row button never takes DOM focus through normal navigation. `empty`,
`loading` — **absent**. `error` — a transient `✕ failed` span owned by the tree,
reachable only by making a real operation fail; **not modelled, not fabricated**.

## ✅ THE GATE PASSES ON ALL 18 RUNNABLE CELLS — 2026-07-31

**3 viewport widths × 2 themes × 3 content lengths = 18 cells. Every one PASS.**
Each is a live WKWebView capture diffed against a live GPUI snapshot by me.

| viewport | short | normal | overflow |
|---|---|---|---|
| **600** dark | PASS | PASS | PASS |
| **600** light | PASS | PASS | PASS |
| **800** dark | PASS | PASS | PASS |
| **800** light | PASS | PASS | PASS |
| **1100** dark | PASS | PASS | PASS |
| **1100** light | PASS | PASS | PASS |

Σ ceil excess tracks the badge variant across the breakpoint — **1.51px at 600**
(narrow badge, 12px text) versus **1.73px at 800/1100** (wide badge, 10px text) —
which is the viewport axis doing real work rather than relabelling.
32 snapshots archived under `native/oracle/runs/matrix/`.

### ⚠️ The state axis is **vacuous on this component** — measured, not assumed

The spec picked `tree-row` believing it "exercises … selection state". **In every
live consumer, it does not.** I checked each of the six §8.3 states:

| state | why it cannot be exercised here |
|---|---|
| `hover` | Reference undrivable. CSS `:hover` is **UA pointer state**; dispatched events do not set it and `webview_interact` has no hover action. Chrome's `CSS.forcePseudoState` would do it — WKWebView exposes no CDP. |
| `focus` | **Paints nothing.** The `:focus-visible` rule is scoped to `.file-tree-container` and `TreeRow` carries `outline-none focus:outline-none`. |
| `selected` | **Never set.** `SidebarTreeRow` accepts an `active` prop, but *neither* live consumer passes it — `git-status-file-item.tsx` contains no reference to `active` at all, and `changed-files-tree.tsx` does not pass it either. `data-active` is dead in every real call site. A real click confirmed it: `activeAfter: 0`. |
| `loading` | No such rendering exists in `GitFileItem`. |
| `error` | No such rendering exists in `GitFileItem`. |
| `empty` | Every fixture row carries an `uncommitted` badge. |

**This is a property of the component, not a limitation of the oracle** — and the
distinction is provable: on the **native** side the state axis demonstrably
works, with `hover` → `#ffffff07` and `selected` → `#ffffff0a`, both distinct
from resting `#00000000`. The machinery measures states correctly. The
*reference* never enters them.

So running the six state flags on this target would produce six identical cells
that converge trivially — the exact "passes while telling us nothing" failure
§8.1 was written to prevent. **I am not counting them as passes.**

**What this means for the STOP gate.** The mechanism is validated on every axis
this component *has*. The state axis needs a target that actually has states —
the file-explorer tree row, which is the same `components/ui/tree-row.tsx` the
spec names, reached through `FileExplorerTreeItem`, and which **does** set
`data-active`. That is the honest way to close the axis, and it is Phase 1 work,
not Phase 2.

## Earlier: the first 3 cells, driven and diffed by me

```
oracle: PASS — 0 deltas over 10 anchors compared,
        5 forgiven by v1.5 content-sizing, 1 forgiven by v1.6 line-sizing
```

| # | viewport | theme | content | depth | anchors | result |
|---|---|---|---|---|---|---|
| 1 | 800 | dark | overflow | 4 | 10 | **PASS** |
| 2 | 800 | dark | short | 2 | 8 | **PASS** |
| 3 | **600** | dark | overflow | 4 | 10 | **PASS** — *below* the 640 breakpoint |

Cell 3 is the one that matters. At a 600px viewport the reference renders the
Badge's **narrow** variant (h20 / 12px) instead of the wide one, and the native
side — which previously had no viewport concept at all — **followed the
breakpoint correctly** via `--viewport-width`. The `state.width` fix is doing
real work, not just relabelling: Σ ceil excess even changes with the variant
(1.73px at 800, **1.51px at 600**), because the badge's text is a different size.

Every difference in all three cells is forgiven by an explicitly **declared**
rule, and each forgiven line names the rule, gives the cause, and quotes **the
tolerance it actually broke** — never a widened one. A PASS reached by forgiving
five comparisons cannot be mistaken for one reached by measuring them.

**What is NOT yet covered**, stated plainly rather than left to inference:

| axis | status |
|---|---|
| viewport widths | 2 of ≥3 (800, 600) — a third still to run |
| theme | dark only — **light not run** |
| content length | short + overflow — **normal not run** |
| `hover` | **cannot be driven on the reference side** (documented above) |
| `focus` | drivable but paints nothing — converges and proves nothing |
| `loading` / `error` | no React original exists; native warns they prove nothing |
| `selected` | not yet attempted — the row's `data-active` is app state, so a real click may drive it |

So: **the gate passes on the cells I have run, and the matrix is not complete.**
Those are different claims and I am not merging them.

## ⚠ OPEN: `state.width` is ambiguous, and the badge proved it is load-bearing

**This already produced one wrong comparison and it will produce more.**

`state.width` is documented only as "integer, logical px". §8.3 asks for "≥3
**viewport** widths". But on the native side there is no meaningful viewport —
the app renders one row — and P1.5's `--width` sets the **surface** width. So
the two sides have been putting different quantities in the same field.

**How it bit.** My first archived reference recorded `state.width: 294` — the
surface width — while the badge's appearance was actually governed by the
**viewport** width, which nothing recorded. The webview happened to be at
**569px**, below Tailwind's 640px `sm:` breakpoint, so the reference rendered
the narrow badge variant (h20 / 12px) while the native side implemented the wide
one (h16 / 10px). The differ dutifully reported four geometry deltas that were
**neither side's fault**. Re-capturing at 1100px made them vanish with no code
change at all.

**Why it recurs.** The §8.3 matrix wants ≥3 viewport widths. Any set that
straddles 640px flips the reference's badge variant — and the native app,
having no viewport concept, **cannot follow**. Every such cell would compare a
narrow-variant reference against a wide-variant native and report deltas that
mean nothing.

**Decision: `state.width` is the VIEWPORT width**, matching §8.3's own wording,
because it is the quantity that drives breakpoint-dependent styling and it is
the one that silently differed. The **surface** width is a separate input.

**What that requires:** the native app must accept a viewport width and size its
window to it, so breakpoint-dependent styling resolves the same way on both
sides — `--viewport-width` alongside the existing `--width`. Until it does, the
three-widths axis of the matrix cannot be run honestly, and **any width cell
that straddles 640px is untrustworthy**.

**Not yet dispatched** — P1.8 currently owns `crowbar-app/src/**`, and a second
worker there would collide. It goes out when P1.8 lands.

> An unrelated confirmation from the same investigation, worth keeping: toggling
> the root `dark` class gives badge background alpha **0.16 dark / 0.08 light**,
> which is exactly the `warning.mix(16 dark / 8 light, TRANSPARENT)` P1.4's
> generator emitted. The light/dark token tables are independently corroborated
> against the live DOM.

## 🎯 THE PHASE 1 GATE — RUN 2: **26 → 18 → 6 deltas**, one row, two root causes

Same cell, everything aligned. **Zero text deltas, zero colour deltas, zero
presence deltas.** All six remaining are geometry, none larger than 1.73px on a
294px row.

```
git-row-name.bounds.w:   103.0, expected 104.73  (Δ -1.73)
git-row-badge.bounds.x:  195.0, expected 196.73  (Δ -1.73)
git-row-name.bounds.h:    19.0, expected  18.0   (Δ +1.00)
git-row-badge.bounds.w:   75.0, expected  74.11  (Δ +0.89)
git-row-added.bounds.w:   12.0, expected  11.16  (Δ +0.84)
git-row-added.bounds.x:  276.0, expected 276.84  (Δ -0.84)
```

**`text_width` is now 476.49 on both sides — exactly zero.** The font fix made
text comparison meaningful rather than accidentally-agreeing.

### The deferred `ceil()` measurement is now answered, and the answer is *better* than feared

| anchor | ref `w` | native `w` | `ceil(ref)` | match |
|---|---|---|---|---|
| `git-row-badge` | 74.11 | **75.00** | 75 | ✅ |
| `git-row-added` | 11.16 | **12.00** | 12 | ✅ |

Both content-sized boxes are **exactly** `ceil(reference)`. And:

```
total ceil excess          = +1.73
git-row-name.bounds.w delta = -1.73     ← the flexible sibling absorbs ALL of it
git-row-added right edge    = 288.00 on BOTH sides — conserved exactly
```

**The excess does not accumulate rightward without bound.** It is absorbed by
the flexible sibling, and the row's total width and the trailing group's right
edge are identical on both sides. The differ worker's worry — that you would
need a tolerance growing with the number of content-sized boxes upstream — does
not materialise, because the layout *conserves* the total rather than
propagating displacement.

**That collapses the modelling problem.** The differ does **not** need flow
order after all:

1. A declared content-sized anchor compares `native.w` against `ceil(ref.w)`.
2. Everything else in the same snapshot gets an additional allowance of
   **Σ(ceil excess) over the anchors declared content-sized in that snapshot** —
   a single global scalar the differ can compute from the anchor list, with no
   tree walk and no flow order.

That is implementable within §1's "no trees" constraint, which is what made the
alternative look expensive.

### The residual: 6 deltas, exactly 2 root causes

| root cause | deltas | magnitude |
|---|---|---|
| GPUI `ceil()`s content-sized box widths | 5 (2 direct, 3 absorbed consequence) | ≤1.73px total |
| GPUI snaps line-height to the device grid | 1 (`name.h` 19 vs 18; 14 × 1.35 = 18.9) | 1.0px |

Both are **framework rounding rules**, both are measured, both are bounded, and
neither is a defect in the ported component. **7 of 10 anchors remain
byte-exact.**

> **Not claiming "converged".** Six deltas exceed ±0.5 and the gate's condition
> is that every anchor converges across the whole §8.3 matrix. What is proven is
> that the *mechanism* works: the oracle found real differences, localised each
> to a specific framework behaviour, and the residual is sub-2px and fully
> explained. That is what Phase 1 exists to establish — but it is not the same
> sentence as "the row converges", and I will not write the second one until it
> does.

## 🎯 THE PHASE 1 GATE — FIRST FULL RUN, 2026-07-31

**The oracle ran end to end.** Live WKWebView snapshot → differ ← live GPUI
snapshot, same matrix cell (`294 · dark · overflow · no flags · depth 4`),
26 ranked deltas. Snapshots archived at `native/oracle/runs/`.

**The mechanism works.** That was the question Phase 1 exists to answer.

### 7 of 10 anchors converged **byte-exactly** — not "within tolerance", identical

| anchor | reference | native |
|---|---|---|
| `git-row-item` | 294×24 @(0,0) | 294×24 @(0,0) |
| `git-row-guide-0..3` | 7×24 @ x=14,28,42,56 | 7×24 @ x=14,28,42,56 |
| `git-row-button` | 294×24 @(0,0) r8 | 294×24 @(0,0) r8 |
| `git-row-icon` | 14×14 @(66,5) | 14×14 @(66,5) |

The row box, every indent guide, the button and the icon land on the same pixel.
The indent arithmetic, the guide geometry and the flex chain all reproduce
exactly. **`git-row-button` radius 8 / border 0 matches on both sides**, which
also closes my earlier stylesheet-reading error for good.

### The 26 deltas, triaged — none of them says "the approach does not work"

**8 are a bug in the differ, not the app.** Every `border.color` mismatch is on
an anchor whose `border.w` is **0**, where the DOM returns inherited text
colour. **ANCHORS v1.3 already rules that colour is compared only when `w > 0`
— the differ predates v1.3 and does not implement it.** Mine to fix; 8 deltas
disappear with it.

**5 are the contract gap P1.5 predicted I would hit first.** `git-row-badge` is
a painted box *containing* text. The React side emits it as a text anchor
(`text`, `fg`, `font`, `text_width`, `clipped`); the native side emits a box.
**v1 has no anchor that is both.** Contract change, not a component fix.

**4 are my own fixture mismatch.** The native row renders `+12`/`-3`; the
reference row renders `+1` and has no deletions. So `git-row-deleted` is
"present natively, absent in reference", `git-row-added.text` is `+12` vs `+1`,
and its width and x cascade from that. **My harness, not the component** — the
two sides must render the same content.

**1 is the decided scope reduction.** `git-row-dir` present natively, absent in
the reference — `showDirectory={false}`, already decided and recorded above.

**That leaves 8 genuine component deltas**, all understood:

| delta | cause |
|---|---|
| `git-row-badge` h 16 vs **20**, and x/y/w | the Badge's `sm:` breakpoint — native implemented the ≥640px variant, the reference is rendering the narrow one |
| `git-row-name.bounds.w` 39.5 vs 91.52, `text_width` 424.05 vs 476.49 | **the font is not loaded** — see below |
| `git-row-name.bounds.h` 19 vs 18 | GPUI snaps line-height to the device grid (14 × 1.35 = 18.9 → 19.0) |

### The state axis works on the native side — verified by me across six cells

The mid-flight correction to P1.5 (visual state must be a **prop**, because the
extractor reads base `StyleRefinement` and cannot see GPUI's runtime `.hover()`)
was necessary and it worked:

| cell | `git-row-item.bg` | `git-row-name.fg` | `text_width` |
|---|---|---|---|
| resting | `#00000000` | `#f5f5f5ff` | 424.047 |
| **hover** | **`#ffffff07`** | `#f5f5f5ff` | 424.047 |
| **selected** | **`#ffffff0a`** | `#f5f5f5ff` | 424.047 |
| focus | `#00000000` | `#f5f5f5ff` | 424.047 |
| **light** | `#00000000` | **`#262626ff`** | 424.047 |
| **short** | `#00000000` | `#f5f5f5ff` | **22.565** |

Hover, selected, theme and content length all move the snapshot. **`focus` is
byte-identical to resting**, exactly as P1.5 predicted — the `:focus-visible`
rule is scoped to `.file-tree-container` and `TreeRow` carries `outline-none`.
So the focus cell will converge and **prove nothing**; that is recorded, not
counted as a pass.

### ⚠ The **hover** cell cannot be driven on the React side with this harness

A real gap, found by trying rather than assuming.

- **Synthetic events do not trigger CSS `:hover`.** `:hover` is driven by the
  user agent's pointer state, not by dispatched `mouseover`/`mouseenter`. A
  `dispatchEvent` changes nothing about which rules match.
- **`webview_interact` has no hover action** — it offers click, double-click,
  long-press, scroll, swipe and focus. None of them leaves the pointer resting
  over an element.
- Chrome DevTools has `CSS.forcePseudoState` for exactly this. **WKWebView does
  not expose CDP**, and the MCP bridge does not reach WebKit's inspector
  protocol.

So the reference row's hover background — which is painted by
`.file-tree-item:hover::before` — is currently **unmeasurable**.

**Three ways out, in order of honesty:**

1. **Move the real cursor.** `CGEventPost` / `CGWarpMouseCursorPosition` from a
   small Swift helper, positioned from the window bounds (already obtainable via
   `CGWindowListCopyWindowInfo`) plus the row's client rect. This produces
   genuine pointer state, so the genuine rule matches. **This is the right
   answer** and it is feasible with tools already proven here.
2. A dev-only route that renders the row with a forced-hover class. Changes the
   app under test, but visibly and in a *test* surface rather than the
   production render.
3. Accept hover as unmeasurable on the reference side and compare the native
   hover against the CSS rule as *specification*. **Weakest** — it tests our
   reading of the stylesheet, which is the exact mistake I already made once.

**Not yet resolved.** It does not block the rest of the matrix, and it matters
again in Phase 5 (interaction and behaviour), so it is written down now.

### The font is the single biggest blocker, and it is an asset problem

`CalSansUI` ships **only as WOFF2** (`web/public/fonts/CalSansUI.woff2`, plus
`CalSans-Regular.woff2` — verified, there is no TTF or OTF anywhere in the
repo). GPUI hands the bytes to CoreText, which rejects them:

```
crowbar-app: font: CalSansUI rejected (parse error)
```

The row still *declares* `font_family("CalSansUI")`, so `font.family` compares
**equal on both sides while the shaping face is silently a fallback**. That is
the worst kind of agreement: the field that would reveal the problem is the one
field that matches.

**Until a non-WOFF2 CalSansUI exists, every `text_width` and every
content-sized `bounds.w` is measuring the wrong typeface.** There is an escape
hatch (`CROWBAR_ROW_FONT=<path to TTF/OTF>`), verified working with a system
TTF.

### DECISION on the GPUI `ceil()` difference — model it, do not loosen; implementation deferred one measurement

**Rejected (a) loosen `bounds.w`.** Not merely because it is lossy. The
differ worker's argument is decisive and I am adopting it: `ceil()` on a
content-sized box **displaces every following sibling's `bounds.x` by the same
amount, cumulatively**, so loosening `w` leaves the downstream `x` deltas firing
and you must loosen `x` too — by a bound that *grows with the number of
content-sized boxes upstream*. **A tolerance that has to grow with the layout is
not a tolerance.** And the error is strictly one-directional (`ceil` can only
make native wider) while a tolerance is symmetric, so half the slack bought is
pure lost coverage bought for nothing.

**Rejected "give the boxes explicit widths so `ceil()` is a no-op"** for
genuinely content-sized anchors. It makes the component worse to make the test
pass: `git-row-added` renders `+1` or `+12`, and a pinned width is wrong at one
of them. Legitimate only where a box is fixed-width *by design anyway* — which
is a component decision, not an oracle one.

**Adopted (b), reframed correctly: this is a correction, not a loosening.** If
GPUI ceils, the native app **cannot produce** a fractional content width, so
asking "is native within 0.5px of WebKit's fraction" asks a question the engine
is incapable of answering — a delta there is never actionable. Comparing against
`ceil(reference)` moves the *expectation* to the one the engine can meet and
keeps the full ±0.5 around it, so a genuine sub-pixel error on a content-sized
box is **still caught**. That is exactly what (a) gives away.

**And: declare the flag, do not detect it.** Detection is a heuristic on both
sides — `width: auto` and not-a-stretched-flex-item on the DOM, `width: None`
plus a text child in GPUI, both falsifiable by flex-grow. Two extractors each
guessing is precisely the silent divergence this contract exists to prevent, and
a mis-guess is invisible: it either opens a 1px blind spot or invents a delta,
and says neither. Content-sizing is a property of the *component*, which already
authors its anchors on both sides — so it becomes an authored argument
(`anchor_content_sized(...)` / `data-oracle-content-sized`). Cost: one optional
boolean in §3, one rule in §5, one argument per affected anchor. A wrong
declaration is then a visible line in a component, not a subtle engine-reading
difference.

**Implementation deferred by exactly one measurement**, and I tried to take it
from the archived run rather than assume:

```
added:  x=252.0  w=21.0  text_width=20.355   ceil excess 0.645
deleted: x=277.0
gap if the ceil propagated     : 4.000
gap if it did not              : 4.645
```

**Inconclusive.** `4.000` is suspiciously exact and I suspect propagation, but
one sample cannot distinguish a 4px gap that inherited the excess from a 4.645px
gap that did not. **P1.7 is adding `--added N` / `--deleted N`**, which lets me
vary the text width and watch whether the downstream `x` moves by the ceil'd or
the raw amount. That settles it.

Why it matters enough to wait: **if displacement propagates, modelling
`bounds.w` alone is not sufficient** and the differ needs flow order to model
`x` — which it deliberately does not have (§1 rejects trees outright). That
would be a genuine design question, not a tweak, and I am not committing to a
shape before knowing which one I am solving.

One supporting fact that makes this modellable at all: **`text_width` needs no
treatment.** GPUI reports the unrounded shaped advance (`20.355`) and it already
compares correctly against WebKit. Only `bounds.w` inherits the ceil — so this is
a box-sizing artefact, not a shaping one.

### One measured difference that exceeds tolerance and is not fixable app-side

GPUI **`ceil()`s a text run's max-content width** — `elements/text.rs`:
`size.width = size.width.max(line_size.width).ceil()`. WebKit keeps fractional
fractions. Measured on a live pair: `git-row-added` has `text_width 20.355` and
`bounds.w 21.0` — **Δ 0.645, against a ±0.5 tolerance.**

It is systematic, it is in the framework, and it hits every content-sized box.
**Not yet resolved.** The options are a modelled comparison (mark content-sized
anchors and compare against `ceil(reference)`) or a looser `bounds.w` tolerance
— and §5 makes loosening a recordable act requiring a stated loss. I am not
loosening it silently.

### ✅ The reference half of the gate is proven, end to end, by me — 2026-07-31

Not a worker's claim. I brought up an isolated reference app, verified its asset
origin, navigated to the fixture workspace, loaded the extractor, and pulled a
real snapshot out of the live WKWebView.

**68 anchors render** on the git status panel: `git-row-item` ×11,
`git-row-button` ×11, `git-row-name`/`icon`/`badge` ×6, `git-row-added` ×4,
`git-row-deleted` ×1, guides 0–3. The panel reads "6 uncommitted changes".

**All three content lengths of the §8.3 matrix are present in one panel** — the
fixture was designed for this and it worked:

| row | file | length |
|---|---|---|
| 2 | `a.ts` | short |
| 10 | `resolve-terminal-connection.ts` | normal |
| 9 | `an-extremely-long-…-sidebar-row.ts` | **overflow** |

**Reference snapshot, row 9 (overflow), 10 anchors:**

```
git-row-item    294×24 @(0,0)   bg #00000000  r 2  bw 0
git-row-button  294×24 @(0,0)   bg #00000000  r 8  bw 0
git-row-icon     14×14 @(66,5)
git-row-name    104.73×18 @(86,3)  fg #f5f5f5ff  14/400/CalSansUI
                text_width 476.49  clipped TRUE      ← 476px of text in a 105px box
git-row-badge   74.11×16 @(196.73,4)  bg #fe9a0029  r 4  bw 1  10/500
git-row-added   11.16×16 @(276.84,4)  fg #00bc7dff  "+1"
git-row-guide-0..3  7×24 @ x=14,28,42,56
```

**This independently confirms my own correction**: `git-row-button` really is
radius **8** with border width **0**, not the 2px/1px I first told P1.5 from
reading the stylesheet. I measured it myself this time.

`git-row-dir` is absent, exactly as P1.1 reported — confirming the
single-span-truncation scope decision above rather than taking it on faith.

**What remains for the gate:** P1.5's native row, then the native snapshot, then
the differ across the matrix.

> **Bridge payload limit.** `webview_execute_js` times out at 7s returning a
> large `JSON.stringify` of a whole snapshot. Return a **trimmed, structured**
> object instead — the data arrives fine that way. Same limit bites a dynamic
> `import()` that makes Vite compile: the import *succeeds*, the call reports a
> timeout, and the module is there on the next call. Do not read either timeout
> as a failure without re-checking.

### ▶ How to bring up the reference app — **do not use `make dev-desktop`**

`make dev-desktop` is wrong for this work, for two reasons that only show up when
more than one Crowbar is running: its `beforeDevCommand` **`pkill -f vite`** kills
every other Vite on the machine — including other agent sessions' and the user's
— and it hard-codes port 5173, which another worktree may already hold.

I did use it twice before understanding that, and it killed a sibling session's
dev server both times. Use this instead:

```sh
# 1. my own Vite, my own port, from MY worktree
cd <worktree>/web && node ./node_modules/.bin/vite --port 5273 --strictPort &

# 2. clear any orphaned daemon on MY socket first (see the spawn trap above)
kill $(pgrep -f crowbar-6d4f21ce150add3c)

# 3. the app, pointed at my Vite, with beforeDevCommand DISABLED so it
#    does not pkill anyone else's
cd <worktree>/desktop
export CROWBAR_HOME="<worktree>/.crowbar"
bunx @tauri-apps/cli dev \
  --config '{"build":{"devUrl":"http://localhost:5273","beforeDevCommand":""}}'
```

The empty `beforeDevCommand` is the courtesy half; the distinct port is the
isolation half. Both are needed.

> **Enumerate your own app instances before launching another one.** A stale
> Tauri instance keeps its window *and its daemon supervisor*, and that
> supervisor keeps trying to spawn a daemon on the same socket. Launching a
> second app then produces:
>
> ```
> ERROR crowbar daemon terminated (code=Some(1))          ← ×4
> ERROR crowbar daemon died 3 times within 600s; giving up until the next app launch
> INFO  crowbar daemon is ready on …6d4f21ce150add3c.sock (pid 18741)
> ```
>
> — a respawn storm ending in "giving up", *immediately followed by* "ready".
> Both lines are true: one daemon did win the socket, and the supervisor has
> permanently stopped covering it for this run.
>
> This looks identical to the start-a-daemon-by-hand trap above and is **not the
> same bug**. Here nobody started a daemon manually; I simply had an old app of
> my own still alive. Killing it then took the winning daemon down with it (the
> window-close kill path), leaving the new app running against nothing.
>
> **Check first**, and attribute by working directory rather than by pid, since
> other sessions' apps look identical in `pgrep`:
>
> ```sh
> for p in $(pgrep -f crowbar-desktop); do
>   lsof -a -p $p -d cwd -Fn | grep ^n | cut -c2-
> done
> ```

**Verified live 2026-07-31:** 5273 serves from my worktree while another
session's instance keeps 5173 and its own daemon (`crowbar-2978a066…`)
untouched. Two Crowbars, two daemons, no interference.

> **Other sessions are running on this machine.** At the time of writing, a
> `feature/crowbar-skill` instance owns 5173 and daemon `24957`. **Do not kill
> it** — it is not mine. Sibling agent sessions share this repo; the workspace
> model memo already warns they will commit into your worktree, and they will
> take your ports too.

### ⚠ PRE-FLIGHT CHECK before every parity run — worker worktrees do **not** isolate ports

**Run this before diffing anything. It takes one command and it prevents a
whole class of silently-invalid comparison.**

```sh
P=$(lsof -nP -iTCP:5173 -sTCP:LISTEN -t | head -1)
lsof -a -p "$P" -d cwd -Fn | grep ^n | cut -c2-      # MUST be <this worktree>/web
```

> **CORRECTION (2026-07-30). My first diagnosis of this was wrong, and the real
> cause is worse.** I attributed it to a worker's `--strictPort` taking the port.
> It is not that. **The repo's own `beforeDevCommand` runs `pkill -f vite`:**
>
> ```json
> "beforeDevCommand": "pkill -f vite 2>/dev/null; sleep 0.5; cd ../web && npm run dev -- --port 5173"
> ```
>
> — `desktop/src-tauri/tauri.conf.json:9`, verified in **this** worktree and in
> `feature/crowbar-skill`'s. It is committed, so every Crowbar worktree has it.
>
> **Consequence: any Crowbar dev instance starting up kills every other Vite on
> the machine** — the orchestrator's reference app and every worker's dev server
> alike. Whoever starts last wins; everyone else's window keeps rendering
> whatever it had loaded, served by nothing, or silently re-attaches elsewhere.
>
> That is the real reason my reference app died twice. The pre-flight check below
> is still exactly right and still mandatory — I just had the mechanism wrong,
> and a wrong mechanism leads to the wrong mitigation. **Re-check the asset
> origin before every parity run, and expect it to have been killed rather than
> merely contended.**

#### What happened, because it will happen again

Workers run in isolated **git worktrees**, which isolates *files*. It does not
isolate *ports* — and, per the correction above, it does not protect a dev
server from being `pkill`ed by an unrelated app start either. The P1.1 worker
legitimately needed a live React app to run its extractor against and started
one from its own worktree.

My reference app's `beforeDevCommand` then failed to bind 5173, `tauri dev`
reported a non-zero exit, and `make dev-desktop` died. **But the Tauri window
launched anyway and attached to the worker's Vite.** Verified:

```
port 5173 pid 16263 cwd = .../.claude/worktrees/agent-<id>/web     ← NOT my worktree
```

So the "reference app" on screen was rendering **a worker's in-progress branch**.
Had I diffed the native app against it, the comparison would have been
meaningless in a way that produces plausible-looking deltas rather than an
obvious error. Nothing in the app, the daemon, or the oracle would have flagged
it.

#### Rules that follow

1. **Verify the asset origin before every parity run**, with the command above.
   "The app is running" is not the same as "the app is mine".
2. A failed `make dev-desktop` **does not mean no app is running** — the window
   can outlive the failure and silently point at someone else's dev server.
3. Do not race a worker for the port. If a worker holds 5173, either wait for it
   or bring the reference up on a different port and point the webview there
   deliberately.
4. The daemon is safe to share — one daemon on the shared `CROWBAR_HOME` is the
   design (§0). It is the **asset origin**, not the daemon, that must be mine.

### The Phase 1 gate fixture — and three traps that cost real time to find

The gate surface is a **git status row**. It cannot render at all unless the
daemon has a project with a repo that has uncommitted changes, and on a fresh
`CROWBAR_HOME` it has none. Setting that up surfaced three things.

**The fixture, now live:**

| | |
|---|---|
| Project | `oracle-fixture` → `/tmp/crowbar-oracle-fixture` |
| Repo | `demo` → `/tmp/crowbar-oracle-fixture/demo`, default branch `main` |
| Origin | `file:///tmp/crowbar-oracle-origin.git` — **a real bare repo, and this is load-bearing** |
| Dirty state | 6 entries: modified ×3, deleted, staged-add, untracked |

Deliberately spans the §8.3 **content-length** axis so truncation is actually
exercised: `a.ts` (short), `resolve-terminal-connection.ts` (normal), and
`an-extremely-long-file-name-that-must-truncate-in-the-sidebar-row.ts`
(overflowing), the last two nested deep enough that the directory span renders.

> **TRAP 1 — repo adoption silently requires a *reachable* remote.**
> `POST /v0/projects/:id/repos` returns **202 Accepted** and then does nothing
> if the repo has no remote, or a remote that cannot be reached. No repo row, no
> error, and **zero log output** — `grep -ci 'import|discover|adopt|provision'`
> over the whole daemon log returns 0. `runAsync` discards the error
> (`if err != nil { return }`).
>
> I lost three attempts to this: no remote → nothing; a plausible-looking but
> non-existent `github.com/...` remote → nothing; a real local bare repo →
> **worked immediately**. Auto-discovery on project import fails the same way,
> which is why importing a project full of local-only repos looks like a no-op.
>
> Discovery itself is fine — `discover.Repos` walks to depth 3 and a `.git`
> *directory* at depth 2 is well within it. The failure is entirely downstream
> in adoption, and it is invisible.

> **TRAP 2 — do not put a fixture inside `CROWBAR_HOME`.**
> `DELETE /v0/projects/:id` **removed the project's `path` directory from disk**,
> taking the fixture and its `.git` with it. The handler's own comment says
> "Real repository directories are never deleted from disk; only
> crowbar-created worktree directories are torn down", and the fixture was at
> `<home>/.crowbar/fixtures/…` — inside Crowbar's own home, which it plausibly
> treats as its to reap.
>
> Stated precisely, because the distinction matters: **repo** delete was
> re-tested afterwards and correctly left the real directory intact. I have
> **not** tested project-delete against a path outside `CROWBAR_HOME`, so this
> is "do not do that", not "project delete destroys your code".

> **TRAP 3 — `git -C <path>` walks up, and it nearly edited the real repo.**
> Running `git -C "$FX/demo" remote add origin …` moments after that directory
> was deleted did not fail — **it operated on the enclosing Crowbar worktree**,
> and was stopped only by the accident that `origin` already existed there. One
> command away from rewriting the real repository's remote.
> **Verify the directory exists before any `git -C`**, or `cd` into it under
> `set -e`. (Crowbar's remotes and working tree were checked afterwards and are
> untouched.)

### Socket-path contract — `crowbar-client` must reproduce this exactly

`api/internal/core/gateway/transports/socket.go:117`. With `CROWBAR_HOME` set:

```
$TMPDIR/crowbar-<fnv1a64(CROWBAR_HOME)>.sock      // Go: fmt.Sprintf("crowbar-%x.sock", h)
```

with no `CROWBAR_HOME`: `~/.crowbar/crowbar.sock`.

> **TRAP — Go's `%x` does not zero-pad.** A hash with a leading zero nibble
> yields a *shorter* filename. There is a live example on this machine right
> now: `crowbar-83bf8eb85db086d.sock` is **15** hex chars, not 16. A Rust port
> written as `format!("crowbar-{h:016x}.sock")` would look correct, pass review,
> and then fail to find the daemon for roughly 1 workspace in 16 — surfacing as
> a bare connection error with nothing pointing at the cause. Use `{h:x}`.
>
> The socket deliberately does **not** live inside `CROWBAR_HOME`: macOS caps
> `sun_path` at 104 bytes and override homes under
> `~/.crowbar/projects/.../worktree` routinely exceed it. Do not "simplify" this.

`desktop/src-tauri/src/sidecar/mod.rs` already carries this fnv1a64 in Rust —
read it before writing a third copy.

---

## Done

### 0.3 — `gpui` + `gpui-component` skills vendored ✅

Merged `754e6c7b`. `.claude/skills/gpui/` (23 upstream files, 7,685 lines) and
`.claude/skills/gpui-component/` (3 files, 861 lines), from
`longbridge/gpui-component` pinned at `88f102d13654fe25aa2fede076274b6b751a3704`
(content revision `a183e4622b099968d9978796e647e6a75f0f5ac1`), Apache-2.0,
`LICENSE-APACHE` + `PROVENANCE.md` carried into each.

**Orchestrator verification** — not taken on the worker's word: `SKILL.md` for
both skills plus `gpui/references/entity-patterns.md` and
`gpui-component/references/usage.md` were re-fetched from
`raw.githubusercontent.com` at the pinned SHA and hashed. All four
byte-identical. Commit scope re-checked: nothing outside `.claude/skills/`.

> **Environment trap this surfaced:** this machine's `~/.gitignore_global` line 2
> is `.claude/`, so the entire vendor was silently unstageable until forced with
> `git add -f`. The repo's own `.gitignore` has no such rule, so CI and fresh
> clones are unaffected — but **anything new added under `.claude/` on this
> machine needs `-f`**, and a worker who does not notice will report success
> having committed nothing.

Not vendored: upstream's third skill `gpui-component-dev` — it is a contributor
workflow for adding components *to* `gpui-component`, which we are consuming, not
contributing to.

### 0.1 — `native/` Cargo workspace scaffold ✅

13 crates per §4.2 (12 under `crates/` + `oracle`), `edition = "2024"`,
`resolver = "3"`, `rust-version = "1.96.0"`, `license = "AGPL-3.0-only"`.

**Orchestrator verification.** I re-ran all four gates myself — `cargo build
--workspace`, `cargo clippy --workspace --all-targets -- -D warnings`, `cargo
test --workspace` (26 binaries), `./scripts/check-invariants.sh` — all exit 0,
zero warnings.

Then, because a checker that only ever prints `ok` is worthless, I **broke it
three ways and confirmed each is caught**:

| Violation introduced | Result |
|---|---|
| `gpui` path dep appended to `crowbar-core/Cargo.toml` | `FAIL rule 1 (§4.3, D2)` |
| `#![forbid(unsafe_code)]` deleted from `crowbar-terminal` | `FAIL rule 2 (§4.3)` |
| `unsafe { }` with no `# Safety` in `crowbar-platform` | `FAIL rule 3 (§4.2, §12)` |

Tree restored clean after each. The worker had done the same, independently —
this is a second, orchestrator-run confirmation, not a repeat of its word.

Two checks are **stronger than the spec asks**: rule 1 also greps
`crowbar-core/src` for `gpui::` (not just the manifest), and rule 2 asserts
`crowbar-platform` positively carries `deny(unsafe_op_in_unsafe_fn)` rather than
merely lacking the forbid.

> **§4.3 rule 3 — token sealing — is NOT yet enforced.** It cannot be: it is a
> property of `crowbar-ui`'s type definitions, and `crowbar-ui` is an empty
> crate. This is the one anti-reward-hacking guard §6.1 calls "strictly
> stronger" than the oracle, and it does not exist yet. **It must land with the
> first line of `crowbar-ui`, in Phase 2, before any component work is
> dispatched** — not after. Tracked here so it cannot be quietly skipped.
>
> **And sealing the newtype is not sufficient on its own.** 0.4 wired `gpui`
> into `crowbar-ui`, which now re-exports it (`pub use gpui;`) so the leaf view
> crates reach it transitively. That means `crowbar_ui::gpui::rgb(0x1e1e1e)` is
> reachable from every view crate, and `.bg(rgb(…))` on a raw gpui element
> bypasses `Theme` entirely — a private inner field on `Color` does nothing to
> stop it.
>
> This hole is **inherent, not caused by the re-export**: no view crate can
> render without gpui's colour types in scope. So the guard has to be a check,
> not a type. `scripts/check-invariants.sh` must grow **rule 4**: no
> `rgb(`/`rgba(`/`hsla(`/`Hsla {` construction anywhere outside
> `crates/crowbar-ui/src/theme/`. Mechanically checkable, and it is the form
> §6.1's claim actually requires.

Known limit, stated in the script's own header: rule 3 is a line scanner, not a
parser. Block comments and string literals containing `unsafe {` produce false
*positives*. It fails loud rather than silent, which is the right direction.

### 0.2 — vendor + pin `gpui` and `gpui-component` ✅ · **the critical path, cleared**

**Pins**, chosen in the required order (gpui-component first, Zed SHA read out of
*its* lock):

| | |
|---|---|
| `gpui` | `zed-industries/zed` @ `1a246efd7e1b83ab568ec5e3e6c1a43a42e1abba` (2026-07-15) |
| `gpui-component` / `-macros` / `-assets` | `longbridge/gpui-component` @ `88f102d1…` (2026-07-30) — same SHA 0.3 used |

**29 crates, 740 files, 238,543 lines, 13.0 MiB.** 21 of the 29 compile on
macOS; the other 8 are carried so manifests stay loadable (Cargo resolves
target-conditional deps for *all* targets, so `gpui_linux`/`gpui_windows`/
`gpui_web` must exist even though we build only macOS).

**Orchestrator verification — my own cold release build:**

```
$ cd native/vendor && cargo build --release -p gpui-vendor-probe --locked
    Finished `release` profile [optimized] target(s) in 6m 41s

├── gpui v0.2.2            (native/vendor/gpui)
├── gpui-component v0.5.2  (native/vendor/gpui-component)
└── gpui_platform v0.1.0   (native/vendor/zed-deps/gpui_platform)
```

`--locked` passing means the committed lock **is** the resolution. Both
libraries resolve from `native/vendor/`, so the collision hazard is structurally
impossible — nothing points at a Zed checkout.

**Shape: extracted subtree with *de-inherited* manifests.** `cargo vendor` was
rejected (drags the entire crates.io graph to disk for 13 MiB of Zed source),
and copying verbatim does not work either — the crates inherit `edition`/`lints`
and ~80 dep specs from Zed's root, and **Cargo refuses a `path` dep into a
nested workspace** (`is a member of the wrong workspace`). Every
`workspace = true` was replaced with its concrete value.

> **The edit that mattered most:** gpui-component's own workspace pinned Zed
> with a **floating** ref — `gpui = { git = "…/zed" }`, no rev. Left alone, our
> "pinned" tree would have silently tracked Zed `main`, which is the exact
> failure §10.5 exists to prevent. Every such entry now points at the vendored
> path.

**`gpui` features confirmed through the resolver, not just the manifest:**
`test-support` → `leak-detection` → `backtrace` (§12 requires leak detection on
in every test). `inspector`, `screen-capture`, `input-latency-histogram`,
`profiler` all present.

Zed pins **Rust 1.95.0** at this SHA — no conflict with our 1.96.0, and its
`rust-toolchain.toml` was deliberately not vendored.

Worth noting: the worker's probe failed its first cold compile on a missing
`use` in **its own probe source**, after all 29 vendored crates had compiled
cleanly, and it recorded that in `PINNED.md` rather than quietly fixing it.

### 0.5 — `protogen`: Go handlers → Rust serde DTOs + TypeScript ✅

Go tool at `native/tools/protogen/`, built on `go/packages` with full type
checking (not regex). **131 types across 22 modules**; 155/161 endpoints fully
resolved; 73/73 request bodies; 74/79 response payloads.

**Orchestrator verification — I ran all four gates:**

| Gate | Result |
|---|---|
| `go test ./...` | `ok …/internal/gen 10.3s` ✅ |
| Generated Rust compiles standalone (temp crate, `serde` only) | `Finished dev profile`, zero warnings ✅ |
| Generated TS typechecks (`--strict`, repo's own `tsc 5.9.3`) | exit 0 ✅ |
| **Determinism** — two full runs, `diff -r` | **byte-identical** ✅ |

**Design calls worth keeping, all verified rather than assumed:**

- **Enums are open sets.** A Go named string type's zero value `""` is legal and
  no constant declares it, so Rust gets an untagged `Other(String)` and TS a
  `(string & {})` member. A closed enum would fail to deserialize valid daemon
  output — a runtime error discovered late, which is exactly what §9.2 exists
  to prevent.
- **Nil slices/maps marshal as `null`, not omitted**, and serde refuses `null`
  for `Vec`/`HashMap`. Non-optional containers deserialize through a generated
  `null_to_default` helper; TS types them `T[] | null`.
- `int64`/`uint64` stay JSON **numbers** — checked, the daemon uses no `,string`
  tags. `time.Time` → `String` (RFC 3339 via its own marshaller, so no `chrono`
  dependency). `[]byte` → `String` (base64).
- **No silent drops.** An unlowerable field gets an `INCOMPLETE:` banner in
  *both* languages and marks every endpoint transitively reaching it as
  unresolved. That is what moved "fully resolved" from 156 down to an honest 155.

**8 diagnostics — the fix list for the Go side, in priority order:**

1. **`gin.H` response payloads (5 endpoints)** — `/v0/health`, `…/identity`,
   `PATCH …/review`, and both `POST …/terminals`. `map[string]any` has no static
   shape. Four return a 1–3 key object, so naming them is cheap.
2. **Anonymous struct nested in a DTO (1)** —
   `UpdateProviderPreferencesRequest.Providers []struct{…}`. A *top-level*
   anonymous body is fine (protogen names it after the handler); one nested
   inside another type has no name to hang on.
3. **`MarshalJSON` on a wire type (1)** — `domain/git.FileDiff`. Its marshaller
   happens to be shape-preserving, but that is unprovable statically.
4. **5 WS routes injected as `gin.HandlerFunc` values** — correctly classified,
   but it means **WS frame shapes are invisible to codegen**. If the port wants
   typed WS frames, that needs a declared handler or a separate declaration.
   Worth knowing now, since the native client is WS-heavy.
5. `libs.Envelope.Data any` is handled, but one hand-rolled
   `c.JSON(status, libs.Envelope{…})` in `identity.go` needed a special case.
   Routing every response through the `libs.Write*` helpers stops that recurring.

### 0.9 — Zed extractability audit ✅ · every verdict compiled, not reasoned about

Audited `zed-industries/zed` @ `b6b2148b` (2026-07-30) in scratch workspaces.

| Crate | Closure | gpui? | Verdict |
|---|---|---|---|
| **`fuzzy_nucleo`** | 17 crates; **3** excl. gpui | 2 methods | **TAKE** |
| **`fuzzy`** | 16; **2** excl. gpui | 2 methods | **TAKE** (dep of the above) |
| **`refineable`** | **0** in-repo deps, 680 lines, Apache-2.0 | **no** | **TAKE**, if we want override layering |
| `util` | 4 crates / 2,819 lines, Apache-2.0 | **no** | Cherry-pick modules only |
| `theme` | 5 excl. gpui | yes | **Skip — on design grounds** |
| `picker` | **90 crates / 500,945 lines** | yes | NOT EXTRACTABLE |
| `editor` | **96 / 518,077** | yes | NOT EXTRACTABLE |
| `language` | **47 / 201,696** | yes | NOT EXTRACTABLE |
| `terminal` | **41 / 193,535** | yes | NOT EXTRACTABLE as-is |
| `ui` (bonus) | 22; 9 excl. gpui | yes, hard | Skip — 95 errors off-rev |

**`fuzzy` is extractable, proven twice.** Its *entire* gpui coupling is two
methods at four call sites: `executor.num_cpus()` and `executor.scoped(…)`. No
`App`, no `Context`, no `Entity`, no rendering. Built and tested unmodified
against stock `gpui 0.2.2` (9/9 tests pass), **and** built with gpui removed
entirely behind a 35-line executor shim (9/9 again, 27 packages total).

**But take `fuzzy_nucleo` instead — it was not on the spec's list and it is the
one we want.** Zed has already migrated off `fuzzy` for exactly our two use
cases; 8 crates now use `fuzzy_nucleo` (`command_palette`, `file_finder`,
`outline`, `tab_switcher`, `git_ui`, …) against 27 still on `fuzzy`. It carries
tuning we would otherwise rediscover — from its own header: nucleo-level
matching must be case-*insensitive*, because `CaseMatching::Smart` **rejects**
candidates whose capitalization differs, breaking a palette lookup of
`"Editor: Backspace"` against an action named `"editor: backspace"`. `Case::Smart`
is honoured as a scoring *hint* instead. Its lib builds clean against 0.2.2; its
23 tests need a matching gpui rev.

**Two prior expectations refuted — same verdict, wrong reason. Recorded so they
are not re-litigated:**

- **`terminal` is not blocked by `project`/`workspace`/`multi_buffer`** — it
  depends on none of them. Its 193k closure is driven almost entirely by
  **`settings` + `theme_settings`** (26 call sites) dragging ~190k lines of
  config plumbing behind a 9,552-line crate. Substituting our own types is
  plausibly a 1–2k-line diff, so this is *not* impossible the way `editor` is —
  but we already ship a working terminal on a daemon-side VT model, so the trade
  is bad. Verdict stands; reclassifiable if anyone wants to pay ~2k lines.
- **`language` is not blocked by `project`/`workspace`/`multi_buffer` either.**
  Its real blockers are `settings`, `lsp`, `rpc` (Zed's collab protocol), `fs`,
  `grammars`, `telemetry`, `zeta_prompt`.

**`picker`'s weld is one line**, which is the interesting part:
`impl<D: PickerDelegate> ModalView for Picker<D> {}` — `ModalView` belongs to
`workspace` (49,498 lines). Only 6 `workspace::` and 2 `project::` call sites in
the whole crate, yet the closure is half a million lines. Take the 134-method
`PickerDelegate` **shape** as a design reference; build on gpui `uniform_list`.

**`theme`: skip, and the reason is our own sealing property.** `ThemeColors` is
**143 `pub` fields, every one a bare `gpui::Hsla`**; `StatusColors` adds 42 more.
~190 colours against our token surface — *not* a superset, so we would be adding
fields, not filling a chassis. Sealing is zero and cannot be retrofitted from
outside: `gpui::Hsla` has public fields and free constructors, so wherever gpui
is in scope `div().bg(rgb(0xff0000))` compiles. Our `Color(Hsla)` would have to
wrap Zed's fields anyway — at which point we have written the struct ourselves
and `theme`'s only residual contribution is the `Refineable` derive. **So take
`refineable` directly** (Apache-2.0, 680 lines, no gpui, no in-repo deps).

What `theme` actually *is*: machinery for **user-authored JSON theme files** —
`ThemeRegistry` loading off a `gpui::AssetSource`, `Option<T>` refinements for
partial overrides. If Crowbar ever ships user-editable themes that is the
argument for revisiting. Otherwise it is dead weight.

**`util`: Apache-2.0, no gpui, 132 tests pass — but do not take `command/`.**
`command/darwin.rs:497` calls `smol::process::Child::adopt_raw_pid`, which
**does not exist upstream** — it is Zed-fork-only and forces their
`async-process`/`async-task` patches into *our* workspace root. Dropping that
module (1,247 of 9,689 lines) removes the requirement entirely. Two more traps:
`util_macros` depends on `perf`, which lives at **`tooling/perf`, not
`crates/`** — a path map built from `crates/*` misses it; and `util` needs
**rand 0.9**, not 0.8.

**Fuzzy-matching fallback, decided now so it is not reopened:** `nucleo` 0.5 /
`nucleo-matcher` 0.3.1, **MPL-2.0**, quiet releases but commits through
2026-06-22 — what Zed itself migrated to. `fuzzy-matcher` (SkimMatcherV2) is MIT
but untouched since 2020-10-04; `skim` is a full TUI and its matcher *is*
`fuzzy-matcher`; `sublime_fuzzy` reports a **non-standard** licence, do not use
without review. Taking `fuzzy_nucleo` gets us nucleo anyway. MPL-2.0 is
file-level copyleft and is standardly read as compatible with (A)GPL-3.0 via
MPL 2.0 §3.3 — fold into the same review as the relicense.

### 0.6 — `GET`/`PUT /v0/settings/ui` ✅ · the ONE daemon exception, and it is closed

1,228 lines: handlers + `domain/ui_settings.go` + routes on `settingsRG` +
399 lines of unit tests + 445 lines of black-box tests in `api/tests/`.

**Orchestrator verification — I drove the endpoint against a live daemon**, on a
scratch `CROWBAR_HOME`, rather than reading the worker's test output:

| Behaviour | Result |
|---|---|
| `GET` a scope never written | `200 {}` — **not** 404 ✅ |
| `PUT` nested object + array + `null`, then `GET` | exact round-trip ✅ |
| Second `PUT` with fewer keys | replaces wholesale ✅ |
| Two scopes | isolated ✅ |
| `PUT [1,2,3]` / `"str"` / `42` / `null` | `400` on all four ✅ |
| `PUT` a 3 MB body | `413`, "exceeds the 256 KiB per-scope limit" ✅ |
| Kill daemon, restart, `GET` | **both scopes survive** ✅ |
| Where it landed | table `ui_settings` **inside `view.db`** — no side file ✅ |

And a detail worth more than it looks: **key order survived the round-trip.**
`{"theme","sidebar","recent","nested"}` came back in insertion order. A Go
`map[string]any` re-marshal would have sorted them alphabetically. So the value
is stored as **raw bytes** — genuinely opaque, exactly as §9.3 demands, rather
than opaque-in-intent-but-parsed-in-fact.

`go test -race` on the new handler package and the black-box suite both `ok`
under my own run.

> **Scope taxonomy — a real discovery, and the store names lie.** §9.3 lists
> four stores as though the split were obvious. It is not:
> `ui-preferences` and `sidebar-ui` are both single **`global`** rows;
> **`workspace-hierarchy` is keyed by REPO id despite its name**; only
> `workspace-layout` is genuinely per workspace. So scope has three forms, and a
> Rust client that trusted the store names would key `workspace-hierarchy`
> wrongly and silently lose hierarchy on every repo with more than one
> workspace.

#### Wire contract — what `crowbar-client` must implement

```
GET /v0/settings/ui?scope=<scope>
  200 {"success":true,"data":{...}}     # {} when nothing stored — NEVER 404
  400 bad or missing scope

PUT /v0/settings/ui?scope=<scope>       Content-Type: application/json
  body: a JSON object, stored verbatim, replaces wholesale
  200 {"success":true,"data":{...}}     # echoes what was stored
  400 non-object body (array/scalar/null/malformed/empty) or bad scope
  413 body > 262144 bytes
  500 store failure
```

`scope` ∈ `global` | `repo:<repoId>` | `workspace:<workspaceId>`. Ids are 1–64
chars of `[A-Za-z0-9._-]`; the whole scope string ≤ 128. Anything else is `400`
on **both** verbs.

Evidence for the three-way split, from `web/src/lib/persistence/`:
`ui-preferences.ts` and `sidebar-ui.ts` both `db.put(…, 'global')`;
`workspace-hierarchy.ts` keys on `repoId`; only `workspace-layout.ts` keys on
`workspaceId`.

#### The 256 KiB cap is load-bearing architecture, not a round number

I verified the reasoning behind it because it encodes a real constraint.
`WorkspaceLayout.buffers` is `PaneContent[]`, and in
`features/panes/types/pane-content.ts`:

```ts
export interface EditorContent extends PaneContentBase {
  content: string        // full file text
  savedContent: string   // ...and a second full copy
```

plus `TokenEntry[]`, a syntax-highlighting cache. **Every open editor tab
persists two complete copies of its file, plus tokens.** That is why today's
persisted layout blob reaches megabytes.

The cap is deliberately set *below* what that blob reaches. So the native client
**cannot** port `workspace-layout` as-is — it is forced to persist *references*
(path, cursor, scroll, pinned/preview flags) and let the daemon own file
content, which is the correct architecture and the one D6 implies anyway.
Treat a `413` here as the design working, not as a limit to raise.

Real payloads for the other three stores are tiny (`UIPreferences` is 6 scalars,
`SidebarUI` two id lists, `WorkspaceHierarchy` `{wsId,parentId}` pairs), so the
cap is ~25× the largest realistic value for them.

#### Concurrency — the mutex is not decorative

Per-scope `sync.Map` of mutexes. GORM's `Save` emits `UPDATE …` and only falls
back to a separate `INSERT` when it matches zero rows, so **two first-writes to
a fresh scope can both see zero rows and both insert.** Removing the mutex makes
`TestPutUI_SameScopeWritesAreSerialised` fail with observed overlap 4. Distinct
scopes never contend — proven by a rendezvous test that deadlocks if they shared
a lock.

#### Known gap, deliberately left

Nothing reaps `workspace:<id>` scopes when a workspace is deleted. Rows are a
few hundred bytes and orphans are inert. `DELETE /v0/settings/ui` is the obvious
follow-up if the cascade should reap them — **not** done, and recorded rather
than silently skipped.

> **Pre-existing red gate, verified by me at HEAD — not inherited from 0.6.**
> `TestRouteAudit_AllSpecRoutesRegistered` (build tag `integration`) fails on a
> clean tree: **161 registered routes vs 159 expected**, from two routes
> registered but absent from the spec list —
> `POST /v0/projects/:projectId/repos/:repoId/workspaces/import` and
> `GET /v0/projects/:projectId/repos/:repoId/pull-requests`. I reproduced this
> before merging anything. `/v0/settings/ui` is **not** among the undeclared
> routes, which is the check that actually matters here.
>
> This is not the port's bug, but it **is** a red gate that will mask a genuine
> route regression from this port later, so it is not something to shrug at. It
> belongs to whoever added those two routes. Filed as
> `native/oracle/blocked/route-audit-red-at-head.md`.

### 0.7 — loopback TCP listener, token-authed ✅

New leaf package `api/internal/core/loopback/` + `transports.NewLoopback`.
**Off by default** (`--loopback-tcp` / `CROWBAR_LOOPBACK_TCP`), so nothing about
today's behaviour changes.

> **What auth existed before: none.** Verified. The unix socket's *entire* access
> control is `chmod 0600` on the socket file
> (`transports/socket.go:52`). `middleware.CORS()` + `origin.go` is a
> **browser-only** control — it decides whether a browser may *read* a
> cross-origin response, and stops nothing for a local non-browser process,
> which sends no `Origin` at all (`Allowed("") == true`). Over TCP there was
> nothing whatsoever. So §9.4's "carry the existing daemon auth" had nothing to
> carry; the token mechanism is new.

**Orchestrator verification — I ran the real binary and inspected the sockets:**

| Check | Result |
|---|---|
| Off by default | no `loopback.json`, **0** TCP listeners on the pid ✅ |
| Bind address (`lsof`) | **`127.0.0.1:53292` only** — no `0.0.0.0`, no `::` ✅ |
| Reachable on the LAN IP `10.10.128.12`? | connection **refused** ✅ |
| `loopback.json` mode | `-rw-------` ✅ |
| Token | 43 chars = 256 bits base64url ✅ |
| no credential / wrong / empty bearer | `401` / `401` / `401` ✅ |
| bearer / `X-Crowbar-Token` / query param | `200` / `200` / `200` ✅ |
| **static asset** and **`/`** with no credential | `401` / `401` ✅ |
| **WS upgrade** with no token | `401` (written *before* the hijack) ✅ |
| **unix socket** with no credential | `200` — unchanged ✅ |
| Non-loopback binds | all 11 subtests refuse, **including `localhost`** ✅ |

Refusing the literal hostname `localhost` is the sharp call and it is right: a
hostname is OS-resolved and `/etc/hosts` can point it off-box. It refuses rather
than warns — the pre-existing `NewTCP` only `slog.Warn`s, and its own doc
comment calls that API "unauthenticated".

**It also fixed a real pre-existing bug while in there.** The listener was bound
*before* the engine/adapter/app wiring, so any failure in that wiring returned
with the listener still open. On the unix path that leaves a socket the next
launch dials successfully and reads as `ErrDaemonRunning` — **a dead daemon
squatting the address**, which is a plausible cause of past "daemon won't start"
reports. Fixed with a named error return + `defer closeOnFailure`, plus
`container.Close()` so half-built PTYs/watchers/sqlite handles aren't stranded.
Regression test: `TestNew_FailureAfterBind_ReleasesTheUnixSocket`.

Nice instinct worth keeping: `api/tests/loopback_tcp_test.go` deliberately
carries **no `integration` build tag**, unlike every other file in that package,
so "who may reach the daemon" is gated on every default `go test ./...` rather
than on a tag nobody runs. Given §0.6's finding that the tagged route audit has
been failing unnoticed, that is exactly the right call.

> **DECIDED (mine, not a user decision): webview panes get a request
> interceptor, NOT a session cookie.**
>
> The problem 0.7 correctly surfaced: a browser *document navigation* and its
> subresource loads cannot set headers, so `webview.load("http://127.0.0.1:PORT/route")`
> 401s on every JS/CSS chunk. Header and query-param auth cover XHR and
> WebSocket (the `WebSocket` API cannot set headers either, which is why the
> query param is load-bearing rather than convenience) — but not chunk fetches.
>
> The obvious fix is Jupyter's: set a session cookie on successful query-param
> auth. **Rejected.** Cookies are not port-scoped — any `127.0.0.1:*` origin
> would receive it. This machine routinely runs several local dev daemons at
> once, so that is not theoretical; it hands our token to any other local
> server's page and reintroduces CSRF surface on state-changing routes.
>
> `wry` is WKWebView on macOS and can intercept/decorate requests, which keeps
> the credential out of any ambient store and is port-scoped by construction.
> **`crowbar-webview` owns this**; it is a Phase 3 implementation detail, not a
> blocker, and not a product decision.

### 0.11 — `cargo tree -i zlog` ✅ · **the alleged GPL chain does not exist in the released crate**

§15 asked for this "for the record", against zed-industries/zed#55470's claim
that `gpui → sum_tree → ztracing → zlog` pulls GPL into a nominally Apache-2.0
crate. Resolved a lockfile for `gpui = "0.2.2"` (**704 packages**) and asked:

```
$ cargo tree -i zlog       → error: package ID specification `zlog` did not match any packages
$ cargo tree -i ztracing   → error: package ID specification `ztracing` did not match any packages
$ cargo tree -i sum_tree   → error: package ID specification `sum_tree` did not match any packages
```

**None of the three is present at all** — not `zlog`, not `ztracing`, and not
even `sum_tree`, the head of the alleged chain. The published crate does not
carry it. (This is expected in hindsight: crates.io requires every dependency to
itself be published, and none of those three are.)

So the concern is void for **Config 1** regardless of the D1 relicense — which
was going to make it moot anyway.

> **UPDATED after 0.2 chose Config 2 — and for the config we actually adopted,
> the chain is REAL.** I re-checked the vendored tree directly:
>
> | Crate | Licence in the vendored tree |
> |---|---|
> | `zlog` | **GPL-3.0-or-later** |
> | `ztracing` | **GPL-3.0-or-later** |
> | `ztracing_macro` | **GPL-3.0-or-later** |
> | `sum_tree` | Apache-2.0 |
> | `path` | **GPL-3.0-or-later** ← 0.9 reported this as Apache-2.0. It is not. |
>
> So zed#55470's `gpui → sum_tree → ztracing → zlog` is genuinely compiled into
> our binary, plus a fourth independent GPL edge via `http_client → util → path`.
>
> **This is legally fine and was anticipated: D1 exists precisely for it.**
> AGPLv3 §13 permits combining with GPLv3. It is not a blocker.
>
> **But it is an obligation, not a non-event.** `NOTICE.md` must list `zlog`,
> `ztracing`, `ztracing_macro` and `path` as GPL-3.0-or-later. The 0.12
> relicense swept our *own* licence surface and deliberately left third-party
> notices alone — correctly, since none of this was vendored yet. It is now.
> **Tracked as a Phase 1 prerequisite.**

### 0.8 — `.app` bundling ✅ · **DECIDED: `cargo-packager`, pinned to `0.11.8`, driven by our own script.**

Not chosen on impressions. The worker installed it and **actually built** a
universal, signed `Crowbar.app` + DMG with a universal sidecar, then inspected
the result: both Mach-Os `x86_64 arm64`, sidecar signed separately with the
hardened runtime (inside-out order), `codesign --verify --deep --strict` valid
and explicitly validating the nested sidecar, `Info.plist` carrying
`CFBundleIconName` for the Liquid Glass icon. Criterion 1 demonstrated.

Why it wins:

- Its signing path **is literally** `docs/macos-code-signing.md`'s:
  `codesign --options runtime --timestamp` → `notarytool submit` →
  `stapler staple`. **5 of the 6 CI secrets are read from identical env vars.**
- macOS DMG naming is **byte-identical to Tauri's** (`Crowbar_{v}_universal.dmg`),
  so `arrow.yaml` and the workflow rename step need no change.
- ~110 lines to maintain vs ~300 hand-rolled, and we keep AppImage/deb for free.
- Same lineage we already ship — `cargo-bundle → Tauri Programme → CrabNebula`.
  The risk is not new; it is the risk we already carry via `tauri-bundler`.

**Zed is the prior art and it argues against hand-rolling.** Zed does *not*
hand-roll: `script/bundle-mac` installs a **Zed-owned fork of `cargo-bundle`**,
and that fork has been *archived since 2025-03-25* while Zed still installs from
it. Their answer to bundler risk was freeze-it-and-own-the-fork — exactly the
escape hatch we keep. Zed also does not ship universal at all (separate
`Zed-x86_64.dmg` / `Zed-aarch64.dmg`, no `lipo`), so their 340 lines are
*less* than our requirement, and the script is welded to Zed-specific machinery.
Not liftable.

`cargo-bundle` upstream was evaluated and **rejected on evidence**: grepping its
`src/` for `codesign|notarytool|stapler|lipo|universal` returns zero hits outside
`hdiutil`. It is the hand-rolled option with a helper.

**Maintenance risk, stated honestly rather than waved away:** no release since
2025-11-27, no commit since 2026-03-21, and its own docs still say "public
preview". It is *slowing badly, not dead* — CrabNebula staff still answer issues,
though a fix promised 2026-04-24 has not shipped. Mitigated by pinning `0.11.8`,
keeping the driving logic in **our** script, and the MIT/Apache-2.0 licence: the
~1k lines of `package/{app,dmg}` + `codesign/macos.rs` are vendorable into
`native/tools/` the day we need them. Not eliminated.

**Five traps found by running it, all of which would have cost real time:**

1. `--config` **replaces**, it does not deep-merge like Tauri's. The current
   script's jq-overlay idiom must become jq-merge-into-a-full-temp-config.
2. `CFBundleVersion` defaults to a UTC **timestamp**, not the version. Must be
   overridden via `infoPlistPath`.
3. **The empty-secret trap survives.** `try_sign` does
   `env::var_os("APPLE_CERTIFICATE")` with no emptiness guard, so an unset CI
   secret expanding to `""` yields `Some("")` and still calls `setup_keychain("")`
   — the exact failure `build-macos-dmg.sh:60-66` exists to prevent. **Carry
   that `unset APPLE_*` block over verbatim.**
4. `create-dmg` is **downloaded at package time** from raw.githubusercontent.
   Its Finder AppleScript fails headless (`AppleEvent timed out -1712`) unless
   `CI=true`. GitHub Actions sets it; a local release build does not.
5. `APPLE_SIGNING_IDENTITY` is the one secret not read from env — it goes
   through config. The existing script already injects it that way.

**Linux artifact names differ** (`crowbar_0.5.0_x86_64.AppImage` vs Tauri's
`Crowbar_0.5.0_amd64.AppImage`), breaking `arrow.yaml:107`. The worker could not
tell from the spec whether Linux stays in scope; **it does** — §14 says GPUI
requires Vulkan and the *React build* is the fallback for machines without it,
which means the native app still ships Linux artifacts for machines with it. So
this rename must be handled.

Also settled, cleanly: **there is no updater to lose.** Zero hits for
`TAURI_SIGNING`, `createUpdaterArtifacts`, `latest.json`, `plugin-updater` or
`checkUpdate` anywhere, and zero `updater` in `Cargo.lock`. Updates go through
Quiver re-downloading the DMG. Dropping Tauri costs nothing here.

### 0.10 — AX spike ✅ · **VERDICT: THIN. Dropped, per §10.4. Do not revisit.**

§10.4 said: spend one hour, and if the tree is thin, "drop it and never revisit."
It is thin. Two disqualifying facts, both read from GPUI source on `main`:

1. **The AX tree is opt-in per element, not the element tree.** A node appears
   only if it has *both* a `GlobalElementId` (`.id()` set on it *and* its whole
   ancestor chain) *and* a non-`None` `a11y_role()`. `_accessibility.rs` states
   it outright: "nodes with no role are not reported." Zed itself annotates
   roughly ten files' worth — button, icon_button, dropdown_menu, keybinding,
   input_field, number_field. Annotating every element of *our* app is the same
   labour as writing the extractor, for a strictly worse data model.
2. **No colour, at all.** AccessKit's node model has no colour concept and the
   GPUI integration writes none. Our oracle diffs geometry **and colour**
   (§8.1). This alone ends it.

The planned feature-gated in-process driver (D7) stands unchanged.

**Two things worth stealing for `crowbar-driver`, and they are worth a lot:**

- **The identity scheme.** `NodeId` is derived deterministically from
  `GlobalElementId::accesskit_node_id()` — the composed path of ancestor ids.
  Stable across frames and across runs *by construction*. That is exactly the
  anchor identity a cross-app differ needs, and it is a solved problem we can
  copy rather than invent.
- **`Window::debug_a11y_tree_json()`** (`window/a11y/debug.rs`, 330 lines,
  `main` only — we get it because 0.2 vendors from `main`). Retains the last
  `TreeUpdate` and serialises on demand, **in-process, with no AX permission**.
  Under `debug_assertions` it additionally records per node the `Render` view
  type name, the originating `ElementId`, whether the node was synthetic, and
  the **`#[track_caller]` source location of the constructing element**. That
  last field is direct precedent for anchoring a UI node back to source. Use
  this module as the template for the extractor's serialisation layer.

Also observed, and useful: `ZED_EXPERIMENTAL_A11Y` **is** honoured by stable
1.6.3 (the binary links `accesskit 0.24.0` / `accesskit_macos 0.26.0` and carries
the tree-builder's log strings) — it is not a `main`-only flag. And `open -na`
does **not** pass environment through; launch `Contents/MacOS/zed` directly.

> **Not completed, stated plainly:** the live AX dump was never produced. Both
> Zed *and* the TextEdit control returned `kAXErrorAPIDisabled (-25211)` with
> `AXIsProcessTrusted() = false` — a macOS TCC permission wall, not an empty
> tree, and the control run is what proves the difference. Finishing it needs a
> human to add a bundle under System Settings → Privacy & Security →
> Accessibility. **I did not ask for that**, because the verdict does not depend
> on it: opt-in annotation and absent colour disqualify the approach whatever
> the live tree looks like. Every point-by-point answer above is source-read,
> not observed, and is labelled as such.

## In flight

**Phase 0 is closed.** All twelve items done and merged, each verified by my own
run rather than a worker's report.

### Phase 1 wave 1 — dispatched 2026-07-30

The contract they all implement is **`native/oracle/ANCHORS.md` v1**, written
before any of them so three independent implementations cannot quietly diverge.

| Item | Branch | Owns | Notes |
|---|---|---|---|
| **P1.1** React extractor | `native/p1.1-react-extractor` | the 9 `data-oracle-id` tags + `web/src/lib/oracle/**` | merged `49ba348f` · live snapshot from the real WKWebView |
| **P1.2** GPUI extractor | `native/p1.2-gpui-extractor` | `crowbar-driver/**` | ✅ **done** — merged `03fb0732` · **STOP-GATE risk retired** · verified by me |
| **P1.3** oracle differ | `native/p1.3-oracle-differ` | `native/oracle/src/**` | ✅ **done** — merged `5fcec61c`, gates re-run by me |
| **P1.4** sealed tokens | `native/p1.4-sealed-tokens` | `crowbar-ui/**`, `check-invariants.sh` | ✅ **done** — merged `60823648`, rule 4 adversarially re-tested by me |
| **P1.5** native row | `native/p1.5-native-row` | `crowbar-ui/src/components/**`, `crowbar-app/src/**` | ✅ merged `11fa277d` — all 5 invariants green |
| **P1.6** differ v1.3 conformance | `native/p1.6-differ-v13` | `native/oracle/src/**` | ✅ **done** — merged `8bee1e23`, **26 → 18 deltas**, verified by my own re-run |
| **P1.7** font + badge | `native/p1.7-font-and-badge` | fonts, `crowbar-ui/components`, `crowbar-app` | ✅ **done** — merged `6af40288`, `text_width` delta **0.000** |

#### P1.2 — the GPUI extractor ✅ merged · **the STOP-GATE risk is retired**

The item most likely to void the spec. **It works, with no fork of
`native/vendor/` and no architectural blocker.** 100% line coverage (1134
lines), 99.83% regions, 63 tests.

GPUI has no post-hoc tree walk and nothing "computed" to read, so the extractor
**participates in the element lifecycle** instead of inspecting it: `anchor()` /
`anchor_root()` / `anchor_text()` are transparent wrappers that return their
child's `LayoutId` from `request_layout`, so GPUI hands them the child's own
bounds at `prepaint`.

Three deliberate choices keep "the driver must not alter rendering" true, and
each is subtle enough to be worth recording:

- `Element::id()` returns `None` — an id would push a `GlobalElementId` and
  **shift every descendant's element-state path**.
- No taffy node of its own. Proven, not asserted: a nested test checks the child
  lands at exactly parent padding + border, which only holds if nothing was
  inserted into the layout tree.
- The registry is read through `try_global`, not `global_mut`, because the
  latter pushes `NotifyGlobalObservers`.

**A real snapshot from the running binary**, with genuine Helvetica shaping:

```
"id": "narrow-label", "bounds": {"x":8,"y":44,"w":90,"h":21},
"text": "resolve-terminal-connection.ts",
"text_width": 173.405, "clipped": true
```

A 173.4px string in a 90px box. **That is exactly the truncation-point
disagreement `bounds` alone cannot see**, and it is why the contract carries
`text_width` at all. The mechanism is doing the job it was designed for.

One colour detail worth keeping: GPUI's `impl From<Rgba> for u32` uses
`(c * 255.0) as u32`, which **truncates** — a channel returning from the HSL
round-trip as `0.99999994` becomes 254, not 255. The float→float half is GPUI's
(so we report what GPUI actually paints); the float→`u8` half is ours and
rounds. Tested across every grey level and a stride of the whole RGB cube.

> **Integration debt, flagged by the worker rather than hidden.** Its test
> surface `crowbar-app/src/driver_surface.rs` fails rule 4 — it constructs
> colours with `rgb(…)`, because it branched before the sealed tokens existed.
> It explicitly did **not** dodge the check with something like `gpui::blue()`,
> which would have passed the letter and defeated the intent. Routed to P1.5,
> which already owns `crowbar-app/src/**`; a second worker there would collide.

#### P1.3 — the differ ✅ merged

100% line coverage (2191/2191), 99.09% regions, 81 tests. No dependencies —
§4.2 gives `oracle` none, so the JSON reader is hand-rolled.

Two design calls worth keeping:

- **Exit 1 and exit 2 are separated deliberately.** 1 means the native app is
  wrong; 2 means *the oracle could not tell you anything* (usage, IO, malformed,
  mismatched matrix cell, empty). Conflating them is how a broken harness gets
  mistaken for a converged one.
- **No `--tolerance-*` CLI flag, on purpose.** §5 requires a loosened tolerance
  to land in its own commit with the measurement justifying it; a flag would let
  the same loosening happen invisibly inside a CI config. `Tolerances` is
  overridable as a library type, where it shows up as a reviewable diff. A test
  asserts five plausible spellings of such a flag are rejected.

Its ranking puts **anchor presence** first and **typography last** — the latter
not because it matters least, but because it is the class most likely to differ
for reasons that are not a Crowbar defect (two font stacks spelling a resolved
family differently), and putting the noisiest class on top defeats the purpose
of ranking.

**It found ten holes in ANCHORS.md** while implementing against it. All ten are
now closed in v1.1, and both extractor workers were sent the delta mid-flight.
That is the contract-first approach working exactly as intended.

#### P1.4 — sealed tokens ✅ merged

**183 fields** (180 from `theme.css` + 3 from the file-tree stylesheet),
**generated, not transcribed**, by a parser that hard-fails on any declaration
it cannot account for or on drift in the 254/180/74 counts — which reproduce my
own measurements exactly.

**Verified against a real browser, not against a second transcription.** A
harness builds a page from the real `theme.css` bytes, asks Chrome for every
token, paints each over white *and* black (unpremultiplied readback destroys
low-alpha colours), and diffs against the constants replayed through gpui's own
`Hsla → Rgba`: **645 samples agree, worst channel delta 1/255.**

Three things that verification caught which would otherwise have shipped wrong:
the app puts `.dark` on `documentElement`, so `:root` and `.dark` are the *same*
element; `@theme inline` aliases substitute at use site, not at the root; and
`oklch(L 0 0)` needs an achromatic short-circuit or matrix noise yields a
garbage hue.

Sealed **tighter than I asked**: `pub(in crate::theme)`, so not even the rest of
`crowbar-ui` can mint a token — only the generated tables and `Color::mix`. And
the reader is deliberately *not* named `hsla()`, which would have collided with
rule 4's own ban.

`theme.accent.mix(68.0, TRANSPARENT) == theme.file_tree_hover_bg` holds exactly
in both appearances — the runtime `color_mix` and the generation-time resolution
are two independent routes to the same float.

Honest about what does not map: `--animate-*` keeps only its duration (easing
and keyframes are gpui code, not tokens), and `--font-editor` has an **empty**
fallback stack because the CSS declares none — Settings supplies it at runtime,
so no static value was invented.

Every brief carries the worker contract: do not run the oracle, do not touch
`native/oracle/corpus/`, do not modify tests you implement against, do not edit
`native/vendor/**`.

**Still not dispatched:** the driver's *input injection*. Driving the native app
into `hover`/`focus`/`selected` needs it, so the gate needs it — but it lives in
`crowbar-driver`, which P1.2 owns right now. It goes out the moment P1.2 lands.
(The React side needs no equivalent: I can dispatch events via `execute_js`.)

**Mine alone, after all four land:** the convergence run across the §8.3 matrix.
That is the gate, and it is not delegable.

## Blocked — needs a user decision

Neither blocks any work. Both are in `native/oracle/blocked/`.

| Item | What is needed | Why it is not mine to decide |
|---|---|---|
| [`cla-policy.md`](oracle/blocked/cla-policy.md) | Whether contributions need a CLA, a DCO, or nothing, now that AGPL-only removes the old rationale | Publishing "no CLA required" is a forward-looking promise to contributors. `LICENSING.md` is left neutral, which reverses cleanly; either answer does not. |
| [`route-audit-red-at-head.md`](oracle/blocked/route-audit-red-at-head.md) | Add two routes to the audit's spec list and bump 159 → 161, or delete them | Two-line fix, but in `api/`, which §0 puts out of scope except for the single §9.3 exception. Reproduced red on a clean tree before any merge. |
| [`vendored-crates-without-a-licence.md`](oracle/blocked/vendored-crates-without-a-licence.md) | Confirm the licence of `gpui_shared_string` and `gpui_util`, or accept that both candidates are compatible | Both are **compiled into our binary** and declare no `license` key — verified absent upstream too, and Zed's root `[workspace.package]` has none to inherit. Either answer is fine under D1, so it is an attribution-accuracy question, not exposure. |

---

## Phase 0 items

Owner column: `W` = dispatched worker, `O` = orchestrator-only.

| # | Item | Spec | Owner | Status |
|---|---|---|---|---|
| 0.1 | `native/` workspace scaffold, 13 crates per §4.2 with the §4.3 compiler-enforced rules | §4.2 §4.3 §4.4 | W | **done** |
| 0.2 | Vendor + pin `gpui` at a SHA under `native/vendor/gpui/` | §10.5 | W | **done — built --locked** |
| 0.3 | `gpui` + `gpui-component` skills into `.claude/skills/` | §16 | W | **done** |
| 0.4 | Both apps launch against one daemon on a shared `CROWBAR_HOME` | §0 §9.1 | O | **done — both apps live on one daemon** |
| 0.5 | DTO generator: Go handlers → `crowbar-proto` + regenerated `web/` types | §9.2 | W | **done — 4 gates re-run** |
| 0.6 | `GET`/`PUT` `/v0/settings/ui` in the daemon — the ONE daemon exception | §9.3 | W | **done — driven live** |
| 0.7 | Loopback TCP listener for webview panes, `127.0.0.1` only, authed | §9.4 | W | **done — driven live** |
| 0.8 | Decide `.app` bundling: `cargo-packager` vs hand-rolled | §14 | W | **done — cargo-packager 0.11.8** |
| 0.9 | Zed extractability audit — `fuzzy`, `picker`, `util`, `theme` | §10.6 | W | **done — take fuzzy_nucleo + refineable** |
| 0.10 | AX spike, timeboxed 1h: `ZED_EXPERIMENTAL_A11Y=1` + an AX tree dump | §10.4 | W | **done — THIN, dropped** |
| 0.11 | `cargo tree -i zlog`, for the record | §15 | O | **done — chain absent** |
| 0.12 | Relicense to AGPL-3.0-only: `LICENSING.md`, `LICENSE`, SPDX, manifests | §15 | W | **done** |

**Phase 0 exit condition — MET 2026-07-30.** All twelve rows `done`.
`cargo clippy --workspace --all-targets -- -D warnings` exits 0 from `native/`,
`cargo test --workspace` is 21/21, and `check-invariants.sh` passes all four
rules. Two items sit in `blocked/` and neither gates anything.

---

## Orchestrator findings — these change how Phase 1 and Phase 2 must be run

Produced by reading the actual Phase 1 gate component and its live CSS, before
dispatching any Phase 1 work. Every one of these would otherwise have been
discovered by a worker mid-item, or worse, not discovered.

### F1 — `tree-row.tsx` alone is not a gate. It is 45 lines and renders nothing.

Spec §16 picks `components/ui/tree-row.tsx` for the Phase 1 gate "because it
exercises text, icons, padding, selection state and truncation in one small
component." It does not. It is a bare `<button>` wrapper that takes `children`;
the text, icons and truncation all live at its call sites.

Gating on `TreeRow` in isolation would converge on *padding-left and a border
radius* and prove nothing — precisely the "the gate could pass while telling us
nothing" failure §8.1 warns about.

It has exactly two real consumers:

| Consumer | Where | Adds |
|---|---|---|
| `SidebarTreeRow` | `components/ui/sidebar-tree.tsx` | `.file-tree-item` wrapper, indent guides, `h-6 gap-1.5 border px-1.5 py-1` |
| `FileExplorerTreeItem` | `features/file-explorer/file-explorer/components/` | the icon + label + status content |

**The Phase 1 gate target is `SidebarTreeRow` as rendered by a real consumer**
(git changed-files or git status), not `TreeRow`. Same component, same size,
but it actually exercises what §16 claims.

### F2 — every visible background on a tree row is painted by a `::before`, and a pseudo-element cannot carry `data-oracle-id`

`features/file-explorer/styles/file-explorer-tree.css`:

```css
.file-tree-item::before            { position:absolute; inset:0; z-index:0;
                                     border-radius:2px; background:transparent }
.file-tree-item:hover::before      { background: var(--file-tree-hover-bg) }
.file-tree-item[data-active=true]::before { background: var(--accent) }
```

while the button itself is pinned `background-color: transparent !important` in
**every** state, hover and selected included.

This is a real hole in the D8 anchored-geometry design: §8.1 has both apps tag
semantic anchors, and **a pseudo-element has no DOM node to tag**. Row
background — hover, active, selection — is the single most visually obvious
thing about a tree row, and it is exactly the thing the oracle as specified
cannot see.

**Resolution (mine, for the Phase 1 extractor):** anchors may be declared
*pseudo-backed*. For those the React-side extractor reads
`getComputedStyle(el, '::before')` — which does return the pseudo's
`background-color` and `border-radius` — and synthesises bounds from the host's
padding box, valid here because the rule is `position:absolute; inset:0`.
The alternative, injecting a real `<div>` under an oracle build flag, is worse:
it changes the app under test. Do not take it.

### F3 — `TreeRow`'s own Tailwind classes are dead. Porting from the class list produces a wrong component.

Inside `.file-tree-container` the cascade overrides nearly all of them:

| `tree-row.tsx` says | What actually renders |
|---|---|
| `rounded-md` (6px) | `border-radius: 2px !important` |
| `hover:bg-muted` | **dead** — `.file-tree-row:hover { background: transparent !important }` |
| `active && bg-accent/20` | **dead** — the `::before` paints full `var(--accent)` |
| `border-none` | `border: 1px solid transparent !important` |

A worker who ports `tree-row.tsx` by reading its `className` — the obvious thing
to do, and what a careful engineer would do in any normal codebase — produces a
component that is wrong in four ways and *looks* faithful.

**This generalises past §6.3.** It is not only transitions and `backdrop-filter`
that must be re-implemented rather than translated: **no value may be taken from
a class list at all.** Everything is measured off the running app. This is the
strongest argument yet for the oracle gating from the very first component, and
it goes in every Phase 2+ worker brief.

> **CORRECTION (2026-07-30) — and the irony is not lost on me. The table above
> does not describe the gate target.** I read the CSS, generalised from it, and
> put the wrong numbers into P1.5's brief. The React extractor then *measured*
> the live element and found:
>
> | Property | I said | Measured on the gate target |
> |---|---|---|
> | `git-row-button` radius | `2px !important` | **8px** (Tailwind `rounded-md`) |
> | `git-row-button` border | `1px solid transparent !important` | **width 0** |
>
> **Why:** those `!important` rules are scoped to
> `.file-tree-container .file-tree-item button`, and the **git status panel's
> rows are not inside `.file-tree-container`** — verified directly,
> `row.closest('.file-tree-container')` is `null`. The 2px/1px treatment is real,
> but only in the *file explorer* tree. Same component, two cascades, and I
> generalised from the wrong one.
>
> The transparency conclusion survives, by a different route: Tailwind
> `bg-transparent` plus the *unscoped* `.file-tree-row:hover`.
>
> So F3's own lesson applied to F3: **I took values from a stylesheet instead of
> measuring, and was wrong in exactly the way F3 warns about.** The correction
> was sent to P1.5 mid-flight, together with the instruction to treat no number
> in its brief as authoritative and let the side-by-side supply the real values.

### F3b — the gate target does not currently exercise the two-span truncation

`git-row-dir` **never renders in the live app.** The only call site of
`GitFileItem` (`changed-files-tree.tsx:222`) passes `showDirectory={false}`.

That matters because "the filename and directory spans truncate against each
other through three nested flex containers" was **my stated reason for choosing
this component as the gate target** (F1). The layout exists in the code and is
worth getting right, but the app does not produce it today, so the gate as it
stands exercises single-span truncation only — which `git-row-name` *does* still
exercise: measured `text_width: 476.49` in a 154.73px box, `clipped: true`.

**DECIDED (2026-07-31): accept single-span truncation for the Phase 1 gate, and
say so — plus carry the two-span case forward to Phase 2.**

Reasoning. Phase 1 exists to validate the *mechanism*: that a driver can extract
post-layout geometry from GPUI, that a React extractor can produce a comparable
snapshot, and that a differ over the two says something true. Single-span
truncation already exercises every part of that — `text_width` against box
width, `clipped`, and the `min-width: 0` chain through nested flex containers.
The measured case is a genuine stress: **476.49px of text in a 154.73px box.**

Two-span truncation is a *harder layout*, not a different mechanism. Forcing it
would mean rendering a configuration the app never renders, which tests a dead
code path and tells us nothing about parity with what users actually see.

**So the Phase 1 gate proves the mechanism on single-span truncation. It does
not prove two-span.** That sentence goes in the Phase 1 report verbatim. The
two-span case becomes a Phase 2 component the moment any surface enables
`showDirectory`.

### F4 — `color-mix(in srgb, …)` is load-bearing and needs an exact implementation

Four distinct uses in this one file, in two different modes:

- `color-mix(in srgb, var(--accent) 68%, transparent)` — mix with `transparent`
- `color-mix(in srgb, var(--accent) 42%, var(--border))` — mix of two opaque colours

Premultiplied-alpha sRGB mixing. A small pure function, no `gpui` types →
**`crowbar-core`**, where the ≥98% gate applies. Not an ad-hoc helper in
`crowbar-ui`.

### F5 — the design-token count is 274 *declarations* but ~180 *tokens*

§3.3's 274 counts declaration lines, which double-counts every token that has a
light and a dark value. Measured in `styles/theme.css`:

| | |
|---|---|
| Declaration lines | 254 |
| **Distinct names** | **180** |
| Declared twice (`:root` **and** `.dark`) | 74 |
| Declared once (theme-invariant) | 106 |

So §6.1's `Theme` struct is **~180 fields of which 74 vary by theme**, not 274
fields. Different shape, roughly half the surface. Plus 21 further custom
properties declared outside the three theme files — of which only ~7 need
porting (`--animate-toast-*` ×4, `--file-tree-guide-icon-offset`,
`--file-tree-hover-bg`, `--tree-guide-color`); the rest are `--md-*` (Plate,
stays webview per §5.3) and Monaco vars (replaced per §5.2).

### F6 — `features/file-explorer/file-explorer/` is the real module; the outer `components/` is a 2-file shim

19 files nested vs 2 outer, and `features/file-explorer/components/file-explorer-tree.tsx`
carries a comment pointing at "the real module under `file-explorer/file-explorer/`".
Every test imports the nested path. Port the nested one; do not be misled by the
shorter path.

---

## Phase 1 — THE GATE (not started)

Build `crowbar-driver` (extract + inject + MCP over stdio) and the
anchored-geometry differ, then converge on one row across the full §8.3 matrix.

### The gate target, made concrete

Per **F1**, `tree-row.tsx` in isolation is not a gate. The target is
`SidebarTreeRow` as rendered by **`features/git/components/status/git-status-file-item.tsx`**.
Same small surface, but it actually exercises what §16 asked for:

| §16 wanted | Where it comes from |
|---|---|
| text | filename span + directory span |
| icons | `FileExplorerIcon`, `SIDEBAR_TREE_ICON_SIZE` = 14 |
| padding | `depth` → `baseIndent 10 + depth × 14` |
| selection state | `.file-tree-item[data-active]::before` (pseudo-backed, see F2) |
| truncation | **both** spans `truncate`, negotiating against each other |

The truncation is the genuinely hard part and the reason this is the right
target. When a directory is shown the filename is `shrink-0 basis-auto
max-w-[45%]` and the directory is `flex-1 truncate`; when it is not, the
filename is `flex-1`. That is a flexbox negotiation whose outcome depends on
`min-width: 0` propagating correctly through three nested flex containers. If
GPUI's taffy layout disagrees with WebKit anywhere, this row is where it shows.

### Anchor set — 9 anchors on one row

| Anchor id | Element | What it proves |
|---|---|---|
| `git-row-item` | `.file-tree-item` | outer bounds; **pseudo-backed** bg for hover + active |
| `git-row-button` | the `TreeRow` button | border, 2px radius, full-width stretch |
| `git-row-icon` | `FileExplorerIcon` | 14px box, `text-muted-foreground` |
| `git-row-name` | filename span | text, `text-foreground`, truncation point |
| `git-row-dir` | directory span | text, `text-muted-foreground/80`, truncation point |
| `git-row-badge` | `Badge variant=warning size=sm` | a nested primitive inside the row |
| `git-row-added` | `+N` span | `text-git-added` |
| `git-row-deleted` | `-N` span | `text-git-deleted` |
| `git-row-guide-<n>` | one per indent level | absolute positioning off a CSS var |

Note the z-order the row depends on: `::before` at `z-0`, the button at `z-2`
(`.file-tree-row`), and every content child explicitly `relative z-1`. Three
layers, and the background is the one that is not a DOM node.

### Exit condition

All 9 anchors converge across ≥3 viewport widths × light/dark × 3 content
lengths × {empty, loading, error, hover, focus, selected}. Convergence on
`git-row-name`/`git-row-dir` must hold at the **overflowing** content length,
where truncation actually engages — a pass that only covers short content is
not a pass.

> **STOP.** If Phase 1 does not converge, the spec is void. Report honestly and
> do not proceed to Phase 2. The implementation plan is written only after
> Phase 1 reports.

---

## Ledger

Append-only. One line per orchestrator iteration.

- `2026-07-30` — Queue created from spec §16. Toolchain blocker resolved
  (`cargo` was installed, just off PATH). Phase 0 wave 1 dispatched.
