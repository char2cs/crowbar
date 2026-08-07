# Rust-Native Desktop Port — Design Spec

**Status:** Approved for execution · **Date:** 2026-07-30 · **Baseline:** `7bb9e829`
**Supersedes:** the draft feature spec of the same date.

> **⚠️ PARTIALLY SUPERSEDED — 2026-08-07.** §11 (the execution loop) and §16 (the
> phase order) are dead, along with the per-component parity verdict as the gate
> for anything. They are replaced by
> [`2026-08-04-slice-based-port-method-design.md`](2026-08-04-slice-based-port-method-design.md),
> which builds the app **one system at a time** — sidebar, tabs, panes,
> settings, terminal, editor, diff, markdown —
> each shipped in the real binary and accepted by the user beside Crowbar-React.
>
> **Everything else in this spec stands.** §4.2 (the crate dependency graph),
> §4.3 (the invariants) and §12 (the coverage gate) are still enforced, by
> `native/scripts/check-invariants.sh` and the workspace lints. Read this file
> for the architecture; read the slice spec for how the work is run.

> This spec is **self-sufficient**. It is written on the assumption that no
> further design conversation happens before the native client is complete.
> Every decision that was open in the draft is closed here. Where a decision
> could reasonably have gone another way, the rejected alternative and the
> reason are recorded, so a future reader does not reopen a settled question.
>
> The implementation plan is a **separate document**, written by a subagent from
> this spec after the Phase 1 gate reports. Do not write it before then.

---

## 0. Scope

Replace Crowbar's Tauri + React desktop frontend with a native Rust application
built on GPUI, talking to the Go daemon.

**Approach: big-bang, dual-app worktree.** Both applications live in the same
worktree and run against the **same daemon instance** via a shared
`CROWBAR_HOME`. The Tauri app remains the daily driver and the live parity
reference. The native app is built alongside it until parity is demonstrated,
then replaces it.

**In scope:** every desktop surface in §5.

**Out of scope:** the Go daemon (`api/`, 164,119 lines), with exactly **one**
scoped exception recorded in §9.3. The browser and Docker delivery surface
(`//go:embed all:web/dist`) is retained permanently (§14).

**Not a concern:** `develop` continuing to receive features and fixes. There is
no frozen baseline. The reference is whatever the worktree holds; drift surfaces
as new oracle deltas like any other gap.

---

## 1. Decisions locked

These were settled in design discussion on 2026-07-30. They are **not open**.

| # | Decision | Consequence |
|---|---|---|
| D1 | **Licence becomes AGPL-3.0-only.** The commercial half is killed. | Zed's GPL-3.0 crates are legally usable. The relicensing door closes permanently once GPL code lands in the tree. Accepted knowingly. |
| D2 | **`crowbar-core` has zero `gpui` dependency. Inviolable.** | Selection logic, tree-expansion state, and similar get pulled *out* of components into core. More ceremony up front; the ≥98% coverage bar becomes reachable rather than aspirational. |
| D3 | **The sidebar scroll-snap carousel is a full parity target.** | Struck from accepted deltas. It must look and behave as it does today. |
| D4 | **The OOBE background becomes the Crowbar motion animation.** | Stays on the accepted-deltas list as an *intentional improvement*, not a downgrade. The old shader gradient is not ported. |
| D5 | **Editor, diff and terminal are improvement targets, not parity targets.** | These three are measured against **Zed** as reference, not against Crowbar-React. Everything else is measured against Crowbar-React. |
| D6 | **No client-side persistence layer at all.** IndexedDB is deleted, not replaced. | `redb`/`rusqlite` struck. Read-through caches deleted outright; local UI state moves daemon-side (§9.3). |
| D7 | **The driver is in-process and feature-gated**, speaking MCP over stdio. | Not an external accessibility-based driver. AX is a spike, not a dependency (§10.4). |
| D8 | **The oracle diffs anchored geometry, not element trees.** | Sidesteps DOM↔GPUI tree non-isomorphism entirely (§10.2). |
| D9 | **Quality bar: no `unsafe` outside one crate; no `unwrap`/`expect`/`todo!` outside tests; ≥98% line coverage on logic crates.** | Enforced in CI, not by review (§12). |

---

## 2. Why

The motivation is not "Rust is faster." It is that a measurable class of our
recorded defects are **webview artifacts, not product bugs**, and are
individually unfixable:

- pane corner composite cost · hidden-sidebar frame stall
- idle-CPU melt · the ProMotion WebGL keepalive (we currently burn GPU on a
  `gl.clear()` loop purely to hold 120Hz)
- `content-visibility` dormancy · Monaco `EditContext` being a no-op in WKWebView
- WKWebView resize re-raster · rAF throttle hiding base-ui overlays
- terminal atlas DPR · terminal glass transparency · terminal italic rendering
- the DOM-renderer tradeoff — we currently choose text quality *or* frame rate

Secondary:

1. **Dev/packaged divergence collapses.** Three shipped PACKAGED-ONLY defects —
   the tree-shaken 0-byte worker chunk, the blank diff in every packaged build
   (#107), agentic chats unopenable under launchd. All from Vite dev-server vs
   static bundle. `cargo build` vs `--release` diverge far less.
2. **The IPC shim is deleted.** `ws_bridge.rs` (601) + `api_proxy.rs` (121) +
   `lib/ws/tauri-transport.ts` + `lib/transport/polyfill.ts` exist *only* because
   a webview cannot open a unix socket.
3. **Dependency surface shrinks** from 84 runtime + 28 dev npm packages to an
   estimated 25–30 crates.
4. **Memory footprint.** A webview process is 200–400MB baseline for an app that
   runs all day beside agent CLIs.

---

## 3. Measured baseline

Measured at `7bb9e829`. Re-measure before quoting these anywhere else.

### 3.1 Frontend

| | Value |
|---|---|
| `web/src` total | 132,886 lines / 1,026 files |
| `web/src/__tests__` | 45,904 lines / 360 files |
| `web/src` non-test | **86,982 lines** |
| Test files | 357 — 119 import `@testing-library/react` (33%), 134 use `vi.mock` |
| npm dependencies | 84 runtime + 28 dev |
| `desktop/src-tauri` | 4,266 lines Rust, **13 IPC commands** |
| `api/` (Go daemon) | 164,119 lines |
| Distinct API endpoint paths | ~~252~~ → **122** (161 method+path pairs) — see below |
| React components | 234 · zustand store files **44** · `useEffect` 229 |

Per-directory, non-test:

| Area | Lines | | Area | Lines |
|---|---|---|---|---|
| `features/editor/` | 25,425 | | `features/tabs/` | 2,184 |
| `components/` | 17,182 | | `features/agent/` | 2,166 |
| `lib/` | 6,930 | | `features/keymaps/` | 733 |
| `features/git/` | 6,295 | | `extensions/` | 589 |
| `features/settings/` | 5,203 | | `styles/` | 580 |
| `features/file-explorer/` | 4,826 | | `utils/` | 540 |
| `features/workspace/` | 4,511 | | `features/window/` | 399 |
| `features/terminal/` | 4,081 | | `features/file-system/` | 349 |
| `features/panes/` | 3,746 | | `routes/`+`hooks/`+`types/` | 310 |

> **Corrected 2026-07-30 (Phase 0). The endpoint count was wrong: 161, not 252.**
> Three independent measurements agree and none reproduces 252:
> 1. `protogen` (item 0.5) walks the gin route registrations and finds **161
>    (method, path) pairs across 122 distinct paths**.
> 2. The daemon's own `TestRouteAudit_AllSpecRoutesRegistered` reports
>    `should have 159 item(s), but has 161`.
> 3. Counting `.GET(`/`.POST(`/… call sites in non-test source under
>    `internal/api/v0` gives **163**, the two-route difference being
>    registration helpers.
>
> `protogen` additionally finds **159/159** of the audit's declared set with none
> missing. **Scope later phases off 161, not 252** — the surface is ~35% smaller
> than the spec claimed.

### 3.2 Component inventory — measured 2026-07-30

| | Count |
|---|---|
| `components/ui/*.tsx` | **72** |
| — of which Plate-only (`block-*`, `*-node.tsx`, `*toolbar*`) | **26** |
| — **requiring a native port** | **46** |
| `components/layout/` | 36 files |
| `components/oobe/` | 1 (`oobe-screen.tsx`) |
| `components/projects/` | 2 |
| `components/editor/` | plugins + `transforms.ts` (Plate — webview) |
| `routes/` | `__root.tsx`, `_shell.tsx`, `_shell/`, `oobe.tsx` |

**46 is the real `components/ui` port target**, not 72. The 26 Plate nodes and
toolbars ship inside the webview surface and are never ported.

### 3.3 Theme tokens — measured

| File | `--` declarations |
|---|---|
| `styles/theme.css` | 264 |
| `styles/zen.css` | 9 |
| `styles/editor-theme.css` | 1 |

**274 total.** The draft said 182; 274 is correct.

### 3.4 Store coupling — measured

| | Count |
|---|---|
| Non-test files referencing a `use*Store` hook | 164 |
| — of which under `components/` | **25** |
| Store definition files | 44 |

Only 25 of 238 `.tsx` files under `components/` touch a store. The
"presentational tier" is real and large; the fan-out in §11 Phase 3 is
justified by this number.

### 3.5 The 13 IPC commands

`ws_open` · `ws_send` · `ws_close` · `terminal_open` · `terminal_send` ·
`terminal_resize` · `terminal_resync` · `terminal_set_theme` · `terminal_close` ·
`reveal_in_finder` · `set_vibrancy_appearance` · `open_window` ·
`diagnostics_export`.

**Nine of thirteen (`ws_*`, `terminal_*`) are deleted, not ported** — they proxy
sockets a webview cannot open. Four survive as `crowbar-platform` functions.

### 3.6 Daemon transport — verified

`api/internal/core/gateway/gateway.go` switches on scheme and supports **both**
`unix://` and `tcp://`. The desktop app runs `unix://` today and reaches it via
`api_proxy.rs` over `UnixStream`. The native client connects to the socket
directly. Webview panes need a loopback TCP listener — see §9.4.

### 3.7 GPUI facts — verified

- `accesskit.workspace = true` in `crates/gpui/Cargo.toml`, **unconditional**,
  not optional, not feature-gated, all platforms.
- `[features]` includes `test-support` (which enables `leak-detection`),
  `inspector`, `screen-capture`, `leak-detection`, `input-latency-histogram`,
  `profiler`.
- Zed ships `ZED_EXPERIMENTAL_A11Y=1` on `main`. Reported state: "some menus,
  some interface elements are accessible but there is a long way to go."
- `docs.rs` exposes **no public accessibility API** on `gpui`. AccessKit is
  plumbing-in-progress, not a contract.

---

## 4. Crate architecture

Crate boundaries follow **testability**, because that is what makes the coverage
bar honest and what keeps worker-agent context small enough to be reliable.

### 4.1 The dependency graph

```
                    Go daemon — unix socket · HTTP + WS
                                  ↕
   crowbar-proto  ◄────────  crowbar-client
   (generated DTOs)          (transport, reconnect, backoff)
        │                            │
        └──────────┬─────────────────┘
                   ▼
             crowbar-core          ── NO gpui. EVER. (D2)
                   │
        ┌──────────┴──────────┐
        ▼                     ▼
  crowbar-state          crowbar-ui   ── Theme + primitives
  Entity<T> + events           │
        └──────────┬──────────┘
                   │
     ┌─────────┬───┴───┬──────────┬──────────┐
     ▼         ▼       ▼          ▼          ▼
 terminal   editor   diff     webview     app
     └─────────┴───────┴──────────┴──────────┘
                   ▼
           crowbar-platform        ── the ONLY unsafe crate

   crowbar-driver  ── taps state + element tree, injects input,
                      speaks MCP over stdio. Feature-gated.
```

> **Corrected 2026-07-30 (Phase 0).** This graph originally drew a chain
> `core → state → ui`. That contradicted §4.2's table, which is the operative
> one, and it was wrong on the merits: it would make every UI primitive link the
> store layer, put `crowbar-ui` out of reach of isolated testing, and invert the
> direction `CLAUDE.md` already mandates ("Stores must not import from
> `components/`"). `ui` and `state` are **siblings on `core`**; the leaf view
> crates depend on both.

### 4.2 Crate contracts

| Crate | Owns | May depend on | Coverage gate |
|---|---|---|---|
| `crowbar-proto` | serde DTOs **generated** from Go handlers | `serde` only | ≥98% |
| `crowbar-client` | socket transport, HTTP, WS, reconnect, backoff | `proto` | ≥98% |
| `crowbar-core` | **all** domain logic: git model, diff algebra, keymap resolution, settings schema + validation, file-tree model, workspace/path scoping, review-thread model | `proto`, `client` | **≥98%, hard fail** |
| `crowbar-ui` | design system: `Theme`, token newtypes, primitives over `gpui-component` | `core`, `gpui`, `gpui-component` | oracle corpus |
| `crowbar-state` | `Entity<T>` stores, event graph, subscription wiring | `core`, `client`, `gpui` | `#[gpui::test]`, ≥90% |
| `crowbar-terminal` | GPU text-grid renderer over the daemon VT model | `ui`, `state` | conformance suite + oracle |
| `crowbar-editor` | `gpui-component` `input` integration, buffers, retained editors | `ui`, `state`, `core` | oracle (ref: Zed) |
| `crowbar-diff` | native review surface, replaces `@pierre/diffs` | `ui`, `state`, `core` | logic ≥98%; view via oracle (ref: Zed) |
| `crowbar-webview` | `gpui-wry` panes and windows | `ui`, `state` | oracle |
| `crowbar-platform` | **the only `unsafe`**: `objc2` vibrancy, `NSWindow`, reveal-in-Finder, `open_window`, `diagnostics_export` | `objc2` | every block proved |
| `crowbar-driver` | element-tree extractor, input injector, MCP stdio server | all | ≥90% |
| `crowbar-app` | binary: window/menu wiring, panes, routing | all | oracle |
| `oracle` | the differ — two JSON snapshots in, ranked deltas out | none | **≥98%** |

### 4.3 Rules the compiler enforces

1. **`crowbar-core` must not list `gpui` in `Cargo.toml`.** CI greps for it. If a
   piece of logic seems to need `gpui`, it is not logic — split it.
2. **`#![forbid(unsafe_code)]` at the top of every crate except
   `crowbar-platform`.** Not a lint; a hard compile error.
3. **Token newtypes are only constructible inside `crowbar-ui`.** `Color`,
   `Space`, `Radius`, `FontSize`, `Duration` have private inner fields and no
   public `from_raw`. A worker agent cannot write `rgb(0x1e1e1e)` at a call site
   and have it compile. This is the anti-reward-hacking guard moved from
   oracle-time to **compile-time**, which is strictly stronger: it cannot be
   argued with and it cannot be missed in review.
4. **`clippy::pedantic` denied workspace-wide.** `unwrap`, `expect`, `panic!`,
   `todo!`, `unimplemented!` denied outside `#[cfg(test)]`.

---

### 4.4 Directory structure

```
crowbar/
├── api/                          164,119 lines Go — UNTOUCHED except §9.3
├── web/                          STAYS PERMANENTLY. Browser + Docker delivery,
│                                 the Linux-without-Vulkan fallback (§14), and
│                                 the target of every webview pane (§5.3).
├── desktop/                      the Tauri reference app. Daily driver during
│                                 the build. Deleted only at Phase 6 sign-off.
├── native/
│   ├── Cargo.toml                workspace root
│   ├── QUEUE.md                  done / in-flight / blocked + both coverage
│   │                             numbers + current phase. How a cold session
│   │                             picks up. Committed every iteration.
│   ├── rust-toolchain.toml       pinned
│   ├── vendor/
│   │   ├── gpui/                 pinned SHA (§10.5)
│   │   └── zed/                  whatever §10.6 finds extractable, GPL
│   │                             headers intact
│   ├── crates/
│   │   ├── crowbar-proto/        generated serde DTOs · zero deps
│   │   ├── crowbar-client/       unix-socket HTTP + WS, reconnect, backoff
│   │   ├── crowbar-core/         ALL domain logic · NO gpui, ever (D2)
│   │   ├── crowbar-ui/           design system · sealed token newtypes (§6.1)
│   │   ├── crowbar-state/        Entity<T> stores + event graph
│   │   ├── crowbar-terminal/     GPU text grid over the daemon VT model
│   │   ├── crowbar-editor/       gpui-component `input` integration
│   │   ├── crowbar-diff/         native review surface
│   │   ├── crowbar-webview/      gpui-wry panes and windows
│   │   ├── crowbar-platform/     THE ONLY unsafe crate
│   │   ├── crowbar-driver/       extractor + injector + MCP  (feature-gated)
│   │   └── crowbar-app/          the binary
│   ├── oracle/
│   │   ├── src/                  the differ
│   │   ├── corpus/               ratcheted action sequences · APPEND-ONLY ·
│   │   │                         workers may not write here (§8.4)
│   │   └── blocked/              items killed after 3 failed attempts (§11.5).
│   │                             One file per item, stating what was tried.
│   ├── tools/
│   │   └── protogen/             Go handlers → crowbar-proto + web/ types
│   └── .claude/skills/           gpui + gpui-component skills, vendored
└── docs/
    └── superpowers/specs/        this spec + the operating prompts
```

---

## 5. Surface assignment

Every surface is assigned to exactly one category and one reference target.

### 5.1 Native, reference = Crowbar-React (strict parity)

Window chrome and vibrancy · tabs bar · **sidebar and project tree, including the
scroll-snap carousel (D3)** · panes and split layout · settings, all tabs ·
command palette · git status / history / branches · agent chat list · file
explorer · OOBE (background per D4) · all 46 non-Plate `components/ui`
primitives (except `dropdown-menu`, per §13) · all 36 `components/layout` files.

### 5.2 Native, reference = Zed (improvement permitted, D5)

| Surface | Base | Notes |
|---|---|---|
| Code editor | `gpui-component` `input` (17,796 lines) | Zed's `crates/editor` is now *legal* (D1) but remains welded to `project`, `workspace`, `multi_buffer`. Extractability, not licensing, is why we use `input`. §10.6 audits this. |
| Diff / review | `crowbar-diff`, native | Replaces `@pierre/diffs`. Removes the draft's §7.3 webview stopgap. |
| Terminal | GPU text-grid over the daemon VT model | Zed's `crates/terminal` is a VT emulator we do not need — the daemon owns the model. `tty7` (Apache-2.0) is the renderer reference. The existing conformance suite remains a hard gate. |

**These three total 35,801 lines — 41% of the non-test frontend — and have no
Crowbar-React reference.** That is the single largest structural consequence of
D5 and it is why the reference target is per-surface rather than global.

### 5.3 Webview (`gpui-wry`)

**Policy — a surface qualifies only if both hold:**

1. The web capability is genuinely irreplaceable in Rust, **and**
2. The surface is a self-contained document that tolerates being an **opaque
   rectangle**.

Clause 2 is not negotiable. `gpui-wry` positions a native OS child view; its
`paint()` renders nothing into the GPUI scene. **Nothing GPUI draws can overlay
it**, and it does not clip to scroll containers — `with_content_mask` there masks
mouse handling only, not pixels.

| Surface | Reach |
|---|---|
| Plate markdown editor | 55 files + 26 `components/ui` nodes/toolbars |
| mermaid | 8 files |
| katex | 4 files |
| HTML preview | already an iframe |

These cost approximately zero new frontend work: the daemon already serves the
React app and we already have URL-scoped routing (`routes/`,
`lib/workspace-scope-url.ts`). A webview pane is `gpui-wry` pointed at
`http://127.0.0.1:PORT/<existing route>` running *the same code through the same
engine* — `wry` is what Tauri uses today. Plate renders **byte-identically to the
current app**, which is the strongest possible form of parity.

**Rich-document surfaces get their own windows, not panes.** This sidesteps the
occlusion class entirely. `open_window` already exists, so the multi-window model
is proven.

A date picker does **not** qualify. Build it.

### 5.4 Deleted, not ported

`ws_bridge.rs` (601) · `api_proxy.rs` (121) · `lib/ws/tauri-transport.ts` ·
`lib/transport/polyfill.ts` · nine of thirteen IPC commands · the ProMotion WebGL
keepalive · `web/scripts/provision-tree-sitter.mjs` and its asset pipeline · the
entry-bundle budget CI gate · **all of `lib/persistence/` (D6)**.

---

## 6. The design system

`crowbar-ui` is the single most important crate for agent-driven throughput,
because it is where correctness becomes a type error rather than a review
comment.

### 6.1 Token typing

The **274** measured CSS custom properties (§3.3) become a `Theme` struct.

```rust
// crowbar-ui/src/theme/token.rs
#[derive(Clone, Copy, PartialEq)]
pub struct Color(gpui::Hsla);          // inner field PRIVATE

impl Color {
    // NO pub fn from_rgb. NO pub const fn new.
    pub(crate) const fn seal(h: gpui::Hsla) -> Self { Self(h) }
}
```

Only `crowbar-ui::theme` may call `seal`. Every consumer writes
`theme.surface.raised`, never a literal. Same for `Space`, `Radius`, `FontSize`,
`Duration`.

**Why this matters more than it looks:** the draft relied on the oracle's state
matrix to catch agents that hardcode offsets to force convergence. That is
detection after the fact. Type-sealing makes the failure *uncompilable*, which
means it never enters the queue at all.

### 6.2 Primitive mapping

Each of the 46 portable `components/ui` primitives maps to exactly one
`crowbar-ui` module. Where `gpui-component` provides an equivalent (dock,
resizable, tree, virtual_list, table, dialog, popover, combobox, menu,
context_menu, native_menu, sidebar, sheet, select, slider, switch, focus_trap,
title_bar, form), we **wrap it** rather than use it directly, so the
`gpui-component` upgrade surface is confined to one crate.

The mapping table is a Phase 2 deliverable and lives in the implementation plan,
not here.

### 6.3 What has no equivalent

`backdrop-filter` and CSS transitions are **re-implemented, not translated**.
Vibrancy is native (`crowbar-platform`). Transitions become GPUI animations with
curves matched by measurement, not by reading the CSS.

---

## 7. State model

44 store files (§3.4) become `Entity<T>` in `crowbar-state`. 229 `useEffect`s
become explicit event wiring.

### 7.1 Rules

1. **Domain state lives in `crowbar-core` as plain Rust.** `crowbar-state` holds
   the `Entity<T>` wrapper and the subscription graph, nothing more. If a store's
   logic can be tested without `gpui`, it belongs in `core` (D2).
2. **No store may depend on a view.** This is today's convention
   (`CLAUDE.md`: "Stores must not import from `components/`") and it carries over
   verbatim.
3. **Per-workspace state stays in a registry**, mirroring
   `features/workspace/stores/`.

### 7.2 The gate

`#[gpui::test]` on a single-threaded `TestDispatcher` with seeded `StdRng`, run
with `iterations = N`. This gives **deterministic, reproducible concurrency
testing** — a capability the React app does not have at all.

Two diagnosed-but-untestable bugs become testable here and **both must have a
regression test before Phase 4 closes**:

- `closeAbandonedTurn` stale `working` read
- the workspace spinner wedge

---

## 8. The parity oracle

### 8.1 Design — anchored geometry (D8)

The draft proposed diffing layout trees. **Rejected.** The DOM and GPUI element
trees are not isomorphic — a sidebar row is six nested `div`s in one and one
element with three children in the other — so a tree differ needs a node
correspondence function that nobody has designed. It would also converge on
`tree-row` by hand-alignment and fail at scale, meaning the Phase 1 gate could
pass while telling us nothing.

Instead:

**Both apps tag semantic anchors.** React uses `data-oracle-id`; GPUI elements
carry an equivalent id. The oracle compares, **per anchor only**:

```json
{
  "id": "sidebar-row-icon",
  "bounds": { "x": 12.0, "y": 84.0, "w": 16.0, "h": 16.0 },
  "text": "feature/foo",
  "fg": "#c8ccd4",
  "bg": "#1e2228",
  "font": { "size": 13.0, "weight": 500 },
  "visible": true,
  "z": 3
}
```

Nesting mismatch becomes irrelevant. Deltas are actionable by construction:
`sidebar-row-icon.bounds.x: 12.0, expected 8.0`.

### 8.2 What the oracle cannot express

GPUI has no "computed style" — it has an already-resolved `Style`. Padding, gap,
size and colour map; `backdrop-filter`, transitions, shadows, blur and vibrancy
do not. Those go to a **secondary perceptual pixel oracle** used *only* for what
anchors cannot express, and its output is a human-triaged score, not an agent
gradient.

Be honest about this: **the primary oracle is a geometry-and-colour oracle.**
That is enough for the 59% of the frontend under strict parity, and it is not
enough for shadows. Do not claim otherwise in any report.

### 8.3 The state matrix — anti-reward-hacking

Every component is gated on a matrix, never a single reference:

> ≥3 viewport widths × light/dark × 3 content lengths (short / long /
> overflowing) × states (empty, loading, error, hover, focus, selected)

State generation reuses `lib/mock/scenarios/{normal,extreme}.ts` and
`lib/store/chaos.ts`.

Combined with §6.1 type-sealing, an agent has no available cheat: it cannot
hardcode colours (won't compile) and cannot hardcode offsets (matrix breaks it).

### 8.4 The corpus is a ratchet

`native/oracle/corpus/` holds the action sequences, checked in.

- **Append-only.** Sequences are never rewritten or deleted.
- **Worker agents cannot touch it.** Enforced by path in the worker contract.
- **Every append is a git-visible admission that a bug escaped the orchestrator.**

This exists because the orchestrator choosing what to compare is the one
self-grading risk the worker/orchestrator split does not remove. Making the
corpus a ratchet converts that risk from invisible to auditable.

---

## 9. Transport and persistence

### 9.1 Client

`crowbar-client` connects **directly** to the daemon's unix socket. HTTP via
`reqwest` with a unix connector, WS via `tungstenite`. No proxy, no bridge, no
polyfill.

### 9.2 DTOs

`crowbar-proto` is **generated** from the Go handlers, not hand-written. This is
a **Phase 0 deliverable**, not a later convenience.

Rationale: there is currently **no codegen anywhere in the repo** and no central
API types module — the 252 endpoints' DTOs are hand-written TS scattered across
features. In TS a drifted field is `undefined` and usually tolerated silently; in
Rust it is a runtime deserialize error discovered only when that surface is
exercised, which under big-bang is late. Generating both sides from one source
turns a late-discovery risk into a compile-time one, and improves the Go side.

The generated TS replaces the hand-written types in `web/` as well, so drift
between the two apps is impossible by construction.

### 9.3 Persistence — the one daemon exception (D6)

**Deleted outright, no replacement** — seven read-through caches:
`workspaces-data`, `git-data`, `file-tree-data`, `branch-review-data`,
`chat-history`, `projects-data`, `chats-data`. The source file itself declares
them "strictly best-effort … must NEVER break the data flow." They exist to paint
before an HTTP round-trip lands; against a local daemon on a unix socket there is
no latency to hide. `Entity<T>` fed by WS is the cache.

**Moved daemon-side** — four stores of real local UI state: `sidebar-ui`,
`ui-preferences`, `workspace-layout`, `workspace-hierarchy`.

> **This is the single scoped exception to §0's "the daemon is untouched."**
>
> Add `GET /v0/settings/ui` and `PUT /v0/settings/ui` — opaque JSON keyed by
> scope, backed by the existing global `view.db` under `<home>/state`. The
> daemon already has a `/settings/*` route group (`agent/routes.go`,
> `terminal/routes.go`) and already persists preferences server-side, so this
> follows an established pattern rather than inventing one.
>
> **Rejected alternative:** a local `$CROWBAR_HOME/native/ui-state.json`. It
> keeps the daemon literally untouched but splits UI state between the native app
> and the web fallback that §14 makes a supported Linux path. Shared state is
> worth one small endpoint.

**Net: the native client has zero local persistence.** `redb` and `rusqlite` are
struck from the library list.

### 9.4 Webview transport

Webview panes load `http://127.0.0.1:PORT/<route>`, which requires the daemon to
listen on TCP as well as the socket. `gateway.New` already supports `tcp://`
(§3.6), so this is a launch-flag change, not new daemon code.

**Bind to `127.0.0.1` only, never `0.0.0.0`**, and carry the existing daemon auth
on that listener. A loopback listener is reachable by every local process; that
is a real exposure and it is not to be waved through.

---

## 10. Libraries

### 10.1 Selected

| Need | Decision |
|---|---|
| Framework | `gpui` — **vendored, pinned to a SHA** |
| Widgets | `gpui-component` + `gpui-component-assets` (Apache-2.0) |
| Editor | `gpui-component` `input` module |
| Syntax | `tree-sitter` via `gpui-component`'s `highlighter` (3,844 lines) |
| Webview panes | `gpui-wry` |
| Transport | `reqwest` (unix connector) + `tungstenite` |
| DTOs | `serde`, generated (§9.2) |
| Persistence | **none** (D6) |
| Vibrancy | our existing `objc2` code, ported from `src-tauri` |
| Diff algorithm | `imara-diff`, unless the daemon already returns unified diff — in which case `features/git/utils/git-diff-parser.ts` ports directly and no algorithm is needed. **Check first.** |
| Fuzzy matching | evaluate Zed's `fuzzy` crate (§10.6) |

### 10.2 Rejected libraries

`git2`/`gix` (the daemon owns git) · `portable-pty` (the daemon owns PTY) ·
`syntect` (tree-sitter matches what we have) · `alacritty_terminal` (the daemon
owns the VT model) · `redb`/`rusqlite` (D6).

### 10.3 Rejected frameworks

Iced (no code editor, no dock, no widget set) · Slint (GPL-or-paid) · egui
(cannot reproduce our design) · Dioxus/Blitz, Freya, Xilem (not
production-ready). There is no versioned, batteries-included alternative; that is
the ecosystem, not a GPUI-specific failing.

### 10.4 The driver (D7)

**No GPUI MCP exists.** Verified — the ecosystem has Appium, browser and TUI
servers and nothing for GPU-native Rust UI. We build one.

`crowbar-driver` is **in-process**, behind `--features driver`, and does three
things:

1. **Extract** — walk the element tree post-layout, emit anchored geometry (§8.1).
2. **Inject** — dispatch keys, clicks, scroll and focus straight into the gpui
   event loop. No coordinate guessing, no timing races.
3. **Serve** — MCP over stdio.

Build the oracle against `--release --features driver` so optimisation level
matches shipping. The driver adds a control channel; it must not alter rendering.
If it ever does, that is a bug of the highest severity.

**Spike, not a dependency: the macOS accessibility route.** Mature generic AX
MCP servers exist (`mcp-server-macos-use`, `Nudge-Server`,
`adamrdrew/macos-accessibility-mcp`) that drive any Mac app via `AXUIElement` and
one of which ships a before/after snapshot diff. GPUI depends on `accesskit`
unconditionally (§3.7). **Spend one hour in Phase 0**: run a Zed nightly with
`ZED_EXPERIMENTAL_A11Y=1`, point an AX MCP at it, record what tree comes back.
If it is rich, it subsumes part of the extractor *and* closes the accessibility
delta in §13.6 as a side effect. If it is thin, drop it and never revisit.

### 10.5 GPUI is pre-1.0 and tracks git main

~~`gpui` is not a released crate.~~ **Vendor it at `native/vendor/gpui/`, pinned
to a SHA.** Zed's churn then reaches us only when we choose to take it.

> **Corrected 2026-07-30 (Phase 0). `gpui` IS a released crate.** Verified
> against the crates.io API: **`gpui 0.2.2`**, licence `Apache-2.0 OR MIT`,
> 177,788 downloads, updated 2025-10-22 (earlier: 0.2.1, 0.2.0, 0.1.0).
>
> The *decision* stands — pin exactly, so Zed's churn arrives only when we take
> it. What changes is that a crates.io version pin is an available and **simpler**
> form of that pin than a vendored git subtree. Two configurations, and item 0.2
> must report on both before we commit:
>
> - **Config 1 — `gpui = "0.2.2"`.** No vendoring, no patches. Cost: ~9 months
>   behind `main`, and `gpui-component` tracks `main`, so the pair may not
>   compile. That is the open question.
> - **Config 2 — pinned git rev.** Compiles by construction, but Zed's workspace
>   carries a global **`[patch.crates-io]` set of 10 forks** (`async-process`,
>   `async-task`, `windows-capture`, `calloop`, `livekit`, `libwebrtc`, `notify`,
>   `notify-types`, `webrtc-sys`, plus font-kit/scap). **`[patch]` applies to the
>   whole consuming workspace**, so those land in `native/Cargo.toml` and affect
>   every crate we build. A `ui`-only build was observed fetching livekit,
>   libwebrtc and the livekit protocol submodule.
>
> **The two cannot be mixed.** Vendoring Zed's support crates while sourcing
> `gpui` from the Zed repo makes cargo refuse outright — *"package collision in
> the lockfile: … only one can be written unambiguously"*. Whatever supplies
> `gpui` must also supply `collections`, `gpui_util`, `refineable` and
> `gpui_macros`.
>
> **Environment:** building any `gpui` on macOS needs the Metal toolchain, which
> Xcode 17 does not install by default (`xcodebuild -downloadComponent
> MetalToolchain`, 688 MB). Absent it, `build.rs` fails with *"cannot execute
> tool 'metal'"*. It **is** present on this machine.
>
> ---
>
> **RESOLVED by item 0.2 — Config 2 (vendored subtree). Config 1 was tried and
> does not compile.**
>
> `gpui = "0.2.2"` **resolves** cleanly and is self-contained (its support crates
> ship as published `gpui_collections`, `gpui_sum_tree`, `gpui_util`, … at
> `^0.2.2`). It simply will not build against `gpui-component`:
> **338 errors across 75 source files** — 139 × `E0599` (`Pixels::as_f32` alone
> at 86 call sites), 56 × `E0308`, 54 × `E0061` arity changes, 33 unresolved
> imports. The decisive one is structural, not drift: **the platform layer has
> moved out of `gpui` into `gpui_platform`/`gpui_macos`/`gpui_linux`/…**, so
> `gpui_platform::application()` does not exist at 0.2.2 at all.
>
> **My `[patch.crates-io]` warning above was wrong, and the worker was right to
> check rather than accept it.** Cargo honours `[patch]` only from the
> **top-level workspace manifest**; a dependency's own patch table is ignored.
> The livekit/libwebrtc fetches observed in §10.6's audit happen when building
> *inside Zed's checkout*, where Zed's manifest **is** the root. Verified against
> our own lock: `livekit`, `libwebrtc`, `webrtc-sys`, `libyuv` are **absent
> entirely**; `async-process`, `async-task`, `calloop`, `notify`,
> `notify-types`, `windows-capture` all resolve from **upstream crates.io**.
> **Zero of the ten forks adopted.**
>
> Four Zed-forked git deps *are* in the lock — `font-kit`, `reqwest`, `scap`,
> `xim-rs` (plus `proptest`), all rev-pinned. Note `font-kit` is a **default
> feature and sits in the macOS dependency block**, so it does build here.
>
> **Re-pin check:** step 8 of `native/vendor/PINNED.md` re-runs the Config 1
> compile. The day it returns zero errors, `native/vendor/` can be deleted in
> favour of a plain version pin.

### 10.6 Zed extractability audit — Phase 0

D1 makes Zed's GPL crates legal. It does not make them extractable. Audit and
record a verdict per crate; take what is genuinely self-contained:

- `fuzzy` — command-palette matching. Likely shallow. **Most promising.**
- `picker` — likely coupled to `workspace`.
- `util` — likely trivial.
- `theme` — evaluate against our own token model (§6.1); probably redundant.
- `editor`, `language`, `terminal` — welded to `project`/`workspace`/
  `multi_buffer`. Expected verdict: not extractable. Record the finding either
  way so it is not re-litigated.

Anything taken lands under `native/vendor/zed/` ~~with its GPL header intact~~.

> **Corrected 2026-07-30 (Phase 0). Zed has no per-file licence headers.**
> Exactly one file repo-wide carries an SPDX identifier. Licensing is
> **per-crate**, expressed twice: a `license = "…"` key in the crate's
> `Cargo.toml`, and a `LICENSE-GPL` / `LICENSE-APACHE` **symlink** in the crate
> directory pointing at the repo root.
>
> So the vendoring rule is: **`cp -RL`** (dereference the symlink so the licence
> text becomes a real file) and preserve the `license =` key. A plain `cp -R`
> yields a dangling symlink and no licence text at all.
>
> Note also that several of these crates are **Apache-2.0, not GPL**: `gpui`
> itself, plus `util`, `path`, `gpui_util`, `collections`, `refineable`,
> `util_macros`, `gpui_shared_string`.
>
> **Audit results are in `native/QUEUE.md` under item 0.9.** Headline: take
> `fuzzy_nucleo` (not `fuzzy` — Zed migrated) and `refineable`; skip `picker`,
> `editor`, `language`, `terminal`, `theme` and `ui`. Two prior expectations in
> the table above were **refuted**: `language` and `terminal` are blocked by
> `settings`, *not* by `project`/`workspace`/`multi_buffer`. Same verdicts,
> different mechanisms — recorded so they are not re-litigated.

> **Correction 2026-08-04 (item 0.13, P3.80). This section's own bullet list
> was incomplete — it never named `git` or `git_ui`, and 0.9 audited exactly
> the list above and nothing more.** That gap went unnoticed until asked about
> directly: why build `crowbar-diff` (§5.2) rather than take Zed's diff
> machinery? 0.13 answers it. Verdict: **none of it is extractable, at any of
> three tiers** — the diff-hunk *model* (`buffer_diff::DiffHunk`) is a
> `gpui::Entity`-backed type welded to `language`'s already-NOT-EXTRACTABLE
> closure, not to `editor`; a *second*, independently NOT-EXTRACTABLE tier
> (`multi_buffer::MultiBufferDiffHunk`) maps hunks into multi-file coordinates;
> and diff *rendering* is welded to `editor` exactly as this section already
> expected, now confirmed with file/line citations rather than assumed. `git_ui`
> itself has a larger closure (115 crates / 764,595 lines) than `editor`. The
> one shallow, `gpui`-light crate found (`git`, blame/commit/remote/stash/status
> porcelain) owns no diff hunks at all and would duplicate what the daemon
> already does. `crowbar-diff`'s scope — data shapes from `crowbar-proto`,
> placeholder hunk-geometry estimation, `patch-window.ts`'s viewport windowing,
> and the review view — has nothing to take from any of them. See `native/QUEUE.md`
> item 0.13 for the closure table and evidence.

---

## 11. The execution loop

### 11.1 Roles

**Orchestrator (the primary agent).** Holds both running apps. Drives them.
Diffs them. Owns the queue and the corpus. **Never writes production code.**

**Workers (subagents).** Receive exactly one item. Write code. **Never grade
their own work. Never run the oracle. Never touch `native/oracle/corpus/`.**

This split exists because an agent that can edit its own gate will edit its own
gate. It is enforced by path restrictions in the worker contract, not by asking.

### 11.2 The item

The unit of work is **closed and binary**. Not "port the sidebar" but:

> `anchor:sidebar-row-icon` converges across the full §8.3 matrix.

or

> `crowbar-core::keymap::resolve` passes the ported test suite at ≥98% coverage.

An item that cannot be phrased this way is not ready to be queued.

### 11.3 Isolation

Every worker runs in **its own git worktree**. Sibling sessions sharing one
worktree have committed each other's work before; a 40-way component fan-out in a
single tree is not a throughput plan, it is a merge accident. Merges are
serialised by the orchestrator.

### 11.4 Test authorship split

Whoever ports a test is **not** whoever makes it pass. Enforced by file glob in
the worker contract, plus a CI check that a test file's hash did not change in
the same commit that implements against it.

### 11.5 Escalation

**Structural, not instructed.** §5.5 of the draft said "agents must fail loudly,
never creatively" — that is a prompt, and prompts do not bind an agent that has
decided it is not stuck.

- The **harness** counts iterations. At **3 failed convergence attempts** on one
  item, the item is killed and written to `native/oracle/blocked/<item>.md`
  stating what was tried and what the residual delta was.
- A blocked item never halts the run. The orchestrator takes the next item.
- `native/oracle/blocked/` is triaged by the orchestrator; anything requiring a
  product decision waits for the user rather than being invented. In particular,
  **adding to the §13 accepted-deltas list is always a user decision.**

### 11.6 Definition of done, per item

1. Compiles with `#![forbid(unsafe_code)]` (or a proved block in `platform`).
2. `clippy::pedantic` clean.
3. No `unwrap`/`expect`/`todo!` outside `#[cfg(test)]`.
4. Coverage gate for its crate met (§12).
5. Orchestrator-run differential comparison green across the §8.3 matrix.
6. No new corpus append was needed to catch a defect in it.

---

## 12. Quality gates

| Crates | Gate | Command |
|---|---|---|
| `proto` `client` `core` `diff`(logic) `oracle` `driver` | **≥98% lines, CI hard fail** | `cargo llvm-cov --fail-under-lines 98` |
| `state` | `#[gpui::test]` on seeded `TestDispatcher`, ≥90% | `cargo test -p crowbar-state` |
| `ui` `terminal` `editor` `webview` `app` | **oracle-corpus coverage, reported separately and never averaged into the line number** | `cargo run -p oracle -- --report` |
| `platform` | every `unsafe` block carries `# Safety`; CI fails if the unsafe line count grows without an accompanying note | `cargo geiger` + custom check |
| all | `#![forbid(unsafe_code)]`, `clippy::pedantic` deny | `cargo clippy --workspace -- -D warnings` |
| leaks | gpui `leak-detection` on in every test; plus an RSS soak against the React app on the same workload | `cargo test --features test-support` |

**Two numbers are reported, always, and never combined.** Line coverage on logic
crates, and corpus coverage on view crates. A single blended figure would hide a
real 20% behind a real 99%, which is exactly the failure mode that made past
coverage numbers untrustworthy.

---

## 13. Accepted deltas — final

"Parity" means parity *modulo this list*. The list is **closed**. Adding to it is
a decision for the user, escalated as such, never a footnote in a report.

1. **Plate, mermaid, katex and HTML preview remain webview.** Indefinitely. They
   render byte-identically to today, so this is a parity *win*, not a gap.
2. **A text-rendering seam exists between native and webview surfaces.** Same
   CoreText rasteriser, different shaping, subpixel quantisation and gamma. Note:
   this is a seam against the *new* chrome, not against the old app — a user
   comparing both apps sees Plate render identically in each. Mitigated further
   by §5.3's separate-window rule.
3. **The OOBE background is the Crowbar motion animation** (D4). Intentional
   improvement.
4. **Linux requires Vulkan** (§14). Users without it fall back to the browser UI.
5. **Accessibility coverage is partial** at parity. `accesskit` is present;
   completeness is not claimed. §10.4's spike may improve this for free.
6. **`backdrop-filter` and CSS transitions are re-implemented, not translated.**
7. **Crowbar-native uses real platform menus**, so a menu does not match
   the React app pixel-for-pixel — it looks like a macOS menu because it is
   one. The concrete consequence: `dropdown-menu` leaves §5.1's strict-parity
   gate. An `NSMenu` is not in the window's view tree and carries no anchor
   id, so the surface is **judged, not diffed** — the same treatment §5.2
   gives the editor, diff and terminal, and for the same reason: no comparable
   reference exists. In exchange: OS keyboard navigation, VoiceOver
   reachability, submenu timing and screen-edge flipping — none of which we
   would otherwise owe an implementation, and §10.4 dropped the accessibility
   spike as "THIN".

**Struck from the draft's list:** the sidebar carousel (now D3, full parity) and
`@pierre/diffs` as a webview stopgap (now native, §5.2).

---

## 14. Distribution

**macOS — unchanged.** Universal binary via `aarch64-apple-darwin` +
`x86_64-apple-darwin`. We lose Tauri's bundler and hand-roll the `.app` or adopt
`cargo-packager` — **decide in Phase 0, do not leave open.** Signing and
notarisation are identical to today's blocked state
(`docs/macos-code-signing.md`) and unblock identically: `codesign --options
runtime --timestamp` → `notarytool submit` → `stapler staple`.

**Linux — a real regression, accepted.** WebKitGTK renders our AppImage and deb
almost anywhere. GPUI requires **Vulkan**; without a compatible GPU the app does
not start (`NoSupportedDeviceFound`). Also requires glibc ≥ 2.29 and X11 and/or
Wayland. Build deps: `libxcb1-dev`, `libxkbcommon-dev`, `libxkbcommon-x11-dev`.
This cuts VMs without GPU passthrough, llvmpipe-only environments, old integrated
graphics and remote-X setups.

**Mitigation, and why `web/dist` stays permanently:** `//go:embed all:web/dist`
is retained regardless. The React build becomes the **supported fallback** for
Linux without Vulkan — same daemon, same URL. This converts a compatibility hole
into a documented path.

The Go sidecar ships alongside exactly as today.

---

## 15. Licensing (D1)

Crowbar becomes **`AGPL-3.0-only`**. The `LicenseRef-Commercial` half is removed.

**Consequences, recorded so they are not rediscovered:**

- AGPLv3 §13 explicitly permits combining with GPLv3. Zed's GPL-3.0 crates are
  therefore usable. `gpui`'s alleged Apache-2.0 contamination via
  `sum_tree → ztracing → zlog` (zed-industries/zed#55470) **stops being a
  blocker**. Run `cargo tree -i zlog` once for the record; it is housekeeping now.
- **The relicensing door closes permanently** the moment GPL code from Zed lands
  in the tree. Even as sole author of our own code, the GPL portion is not ours
  to relicense. This was accepted explicitly (D1).

**Files to change:** `LICENSING.md`, `LICENSE`, SPDX headers, `Cargo.toml` and
`package.json` license fields.

---

## 16. Phases and gates

**Phase 0 — scaffolding and the cheap answers.**
Create `native/`. Vendor and pin `gpui`. Add `gpui-component`'s `skills/gpui/`
and `skills/gpui-component/` to `.claude/skills/`. Prove both apps launch against
one daemon on a shared `CROWBAR_HOME`. Build the §9.2 DTO generator. Add
`GET/PUT /v0/settings/ui` (§9.3). Decide `.app` bundling (§14). Run the §10.6 Zed
extractability audit and the §10.4 AX spike. Run `cargo tree -i zlog` for the
record.

**Phase 1 — the driver and oracle spike. THE GATE.**
Build `crowbar-driver` (extract + inject + MCP) and the anchored-geometry differ.
Prove convergence on **`components/ui/tree-row.tsx`** across the full §8.3
matrix — chosen because it exercises text, icons, padding, selection state and
truncation in one small component.

> **STOP.** If Phase 1 does not converge, this spec is void and the fallback is
> incremental native surfaces hosted inside the existing Tauri shell. The
> implementation plan is written only after Phase 1 reports, scoped from its real
> numbers rather than from estimates.

**Phase 2 — hardest representative components.**
`tree-row`, `dropdown-menu`, `resizable`, `sidebar-carousel`. Front-load
difficulty to establish house style and the §6.2 mapping table early. Throughput
on the tail depends on getting these right.

**Phase 3 — parallel throughput.**
Tier A (`core`, `proto`, `client`, theme tokens — gated by ported tests) and
Tier B (the 46 `components/ui` primitives and 36 `components/layout` files —
gated by the oracle) fan out widely. Justified by §3.4: only 25 of 238 `.tsx`
under `components/` touch a store.

**Phase 4 — state model.**
44 stores → `Entity<T>`; 229 `useEffect`s → event wiring. Gated by `#[gpui::test]`
plus **differential behaviour comparison** — drive both apps through the same
action sequence, compare resulting observable state. The compiler proves it
typechecks; only the differential run proves the reactive graph is right.
`closeAbandonedTurn` and the spinner wedge must both have regression tests before
this phase closes (§7.2).

**Phase 5 — interaction and behaviour.**
Focus order, chords, drag thresholds, IME, scroll, animation curves. Extend the
oracle with interaction record/replay: recorded event sequences → resulting
anchored geometry, replayed against the native build.

**Phase 6 — parity sign-off, then migrate.**
Delete `desktop/`. Not before.

---

## 17. What "done" means

The native client is done when **all** hold:

1. Every anchor under strict parity (§5.1) converges across the §8.3 matrix.
2. Every surface under §5.2 has been judged against Zed and signed off.
3. Both coverage numbers (§12) are met and reported separately.
4. Zero `unsafe` outside `crowbar-platform`; every block there is proved.
5. Zero `unwrap`/`expect`/`todo!` outside tests.
6. The leak soak shows no RSS growth against the React app on the same workload.
7. `blocked/` is empty, or every remaining item is a user decision, listed.
8. The terminal conformance suite is green.
9. A user handed both apps cannot tell which is which, except for §13.

Anything short of this is not done. A verification gap is work remaining, not a
caveat to hand back.
