# Port Queue

Source of truth for the Rust-native GPUI port. Spec:
`docs/superpowers/specs/2026-07-30-rust-native-desktop-port-design.md`.
Updated every orchestrator iteration. This file is how a cold session picks up.

**Phase:** 0 — scaffolding and the cheap answers
**Line coverage (logic crates):** n/a — no logic crates exist yet
**Corpus coverage (view crates):** n/a — no oracle exists yet

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
| Zed | `/Applications/Zed.app` present (stable channel) — used by the §10.4 AX spike |
| Network | reachable |
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

### 0.4 status — React half **verified live**, native half blocked on 0.2

Observed, not inferred. `make dev-desktop` built and launched; the daemon log
shows `crowbar daemon is ready on …crowbar-6d4f21ce150add3c.sock (pid 62909)`,
the daemon binary is the one built from **this** worktree, and `.crowbar/`
now holds `bin/ logs/ state/`. `GET /v0/health` over the socket returns
`{"pid":62909,"status":"ok"}`.

The native half cannot be proven until `crowbar-app` can open a socket, which
needs 0.2. **0.4 stays open.**

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

Known limit, stated in the script's own header: rule 3 is a line scanner, not a
parser. Block comments and string literals containing `unsafe {` produce false
*positives*. It fails loud rather than silent, which is the right direction.

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
was going to make it moot anyway. It remains an open question for **Config 2**
(git-rev gpui), where the in-repo `sum_tree` *is* reachable; item 0.2 should
re-run this if it lands on Config 2.

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

Wave 1 was ten workers, dispatched 2026-07-30, disjoint owned paths, one git
worktree each. Nine have returned, been verified by me, and merged. **One
remains, and Phase 0 now hangs entirely on it.**

| Item | Branch | Owned paths | State |
|---|---|---|---|
| **0.2 vendor gpui** | `native/0.2-vendor-gpui` | `native/vendor/**` | **in flight — the critical path** |

**0.2 was re-briefed mid-flight.** After 0.9 finished, I sent the worker three
findings that change its solution space and asked it to report on **both**
configurations rather than only the vendored-subtree one:

1. `gpui` **is** a released crate (`0.2.2`, Apache-2.0 OR MIT) — its brief and
   §10.5 both said otherwise.
2. A git-rev pin drags Zed's **10 global `[patch.crates-io]` forks** into our
   workspace root, because `[patch]` is workspace-wide.
3. The two sources **cannot be mixed** — cargo refuses with a lockfile package
   collision if vendored support crates coexist with repo-sourced `gpui`.

Expected answer shape: does the chosen `gpui-component` pin compile against
`0.2.2`? If yes, Config 1 wins outright. If no, we need Config 2's real cost
(the exact patch set, and which of those forks actually *build* rather than
merely resolve) written down before accepting it.

Everything still open depends on this: **0.4**'s native half needs a
`crowbar-app` that can open a socket, and the whole of Phase 1 needs a
compiling GPUI.

## Blocked — needs a user decision

Neither blocks any work. Both are in `native/oracle/blocked/`.

| Item | What is needed | Why it is not mine to decide |
|---|---|---|
| [`cla-policy.md`](oracle/blocked/cla-policy.md) | Whether contributions need a CLA, a DCO, or nothing, now that AGPL-only removes the old rationale | Publishing "no CLA required" is a forward-looking promise to contributors. `LICENSING.md` is left neutral, which reverses cleanly; either answer does not. |
| [`route-audit-red-at-head.md`](oracle/blocked/route-audit-red-at-head.md) | Add two routes to the audit's spec list and bump 159 → 161, or delete them | Two-line fix, but in `api/`, which §0 puts out of scope except for the single §9.3 exception. Reproduced red on a clean tree before any merge. |

---

## Phase 0 items

Owner column: `W` = dispatched worker, `O` = orchestrator-only.

| # | Item | Spec | Owner | Status |
|---|---|---|---|---|
| 0.1 | `native/` workspace scaffold, 13 crates per §4.2 with the §4.3 compiler-enforced rules | §4.2 §4.3 §4.4 | W | **done** |
| 0.2 | Vendor + pin `gpui` at a SHA under `native/vendor/gpui/` | §10.5 | W | todo |
| 0.3 | `gpui` + `gpui-component` skills into `.claude/skills/` | §16 | W | **done** |
| 0.4 | Both apps launch against one daemon on a shared `CROWBAR_HOME` | §0 §9.1 | O | React half **verified live**; native half gated on 0.2 |
| 0.5 | DTO generator: Go handlers → `crowbar-proto` + regenerated `web/` types | §9.2 | W | **done — 4 gates re-run** |
| 0.6 | `GET`/`PUT` `/v0/settings/ui` in the daemon — the ONE daemon exception | §9.3 | W | **done — driven live** |
| 0.7 | Loopback TCP listener for webview panes, `127.0.0.1` only, authed | §9.4 | W | **done — driven live** |
| 0.8 | Decide `.app` bundling: `cargo-packager` vs hand-rolled | §14 | W | **done — cargo-packager 0.11.8** |
| 0.9 | Zed extractability audit — `fuzzy`, `picker`, `util`, `theme` | §10.6 | W | **done — take fuzzy_nucleo + refineable** |
| 0.10 | AX spike, timeboxed 1h: `ZED_EXPERIMENTAL_A11Y=1` + an AX tree dump | §10.4 | W | **done — THIN, dropped** |
| 0.11 | `cargo tree -i zlog`, for the record | §15 | O | **done — chain absent** |
| 0.12 | Relicense to AGPL-3.0-only: `LICENSING.md`, `LICENSE`, SPDX, manifests | §15 | W | **done** |

**Phase 0 exit condition:** every row above is `done` or written to
`native/oracle/blocked/`, and `cargo clippy --workspace -- -D warnings` is clean
from `native/`.

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
GPUI's taffy layout disagrees with Blink anywhere, this row is where it shows.

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
