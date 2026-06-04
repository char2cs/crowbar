# WAVE 6 — Tauri + Web + Backend: wire all three, e2e + pressure test (driver agent)

You are getting the **whole Crowbar app working end-to-end** for the first time:
the Go backend (Waves 0–5), the **web app** (`web/`), and the **Tauri desktop
shell** — all three running together, talking to each other, then **heavily
tested end-to-end** with the **`chrome-devtools` MCP server**. This is where the
frontend stops talking to mocks and starts talking to the real `/v0` API and WS
topics.

This is a long, iterative wave. Work in three phases: **Wire → Run → Pressure-test.**

## ⛔ Standards — NON-NEGOTIABLE
- **Frontend code** follows the project's `web/CLAUDE.md` conventions: kebab-case
  component files, narrow Zustand selectors, `@/` imports, tests mirrored under
  `web/src/__tests__/`. No legacy migration code (pre-production — clear dev
  IndexedDB instead).
- **Any Go you touch** follows the full Rabbyte standards (`docs/prompts/README.md`):
  one parameter per line, early returns, ≤3 nesting, one test per source, **no
  `time.Sleep` in tests**, ≥95% coverage.
- **No flaky tests** anywhere — e2e flows must be deterministic (wait on
  conditions/DOM/network, never fixed sleeps).
- **Heavy testing is the point.** Crowbar is an IDE; it must be fast and not leak.
  Measure, don't assume.

## Phase 1 — Wire (frontend ↔ backend; backend ↔ Tauri)
1. **Audit the current data layer** in `web/src`: find every place that hits a
   mock (MSW handlers, fixture imports, stubbed stores). Inventory the API + WS
   surface the frontend actually consumes vs. the real `/v0` routes in
   `api/docs/specs/v0/02-api-surface.md`.
2. **Replace mocks with a real client**: point the API client at the backend's
   `/v0` REST routes; wire the seven WS topics (workspaces, chats, git, files,
   lsp, terminal — chatStream is post-spike) into the store slices, honoring
   **snapshot-on-subscribe** so the sidebar renders live on first connect.
   Reconcile any shape mismatch *against the spec* (the spec is canonical; if the
   frontend expects something undefined, flag it).
3. **Tauri shell**: wire the desktop app to launch/serve the backend (sidecar or
   embedded), bind localhost, and load the web app. Use the `tauri` MCP tools to
   drive/inspect the desktop window where useful.
4. **First-run**: `~/.crowbar` bootstrap + config; graceful messaging when `git`,
   `gh`/`glab`, or language servers are absent.

## Phase 2 — Run (get all three green together)
- Backend server up; web app served; Tauri window loads it; a real project
  imports → repos discovered → workspace opens → file tree + editor + git +
  terminal all functional against real systems. Fix the integration breaks (they
  will be in shape/scoping/timing).

## Phase 3 — Pressure-test end-to-end (chrome-devtools MCP)
Drive the running app through the `chrome-devtools` MCP and test **hard**:
- **Functional e2e walkthroughs**: open file → edit → save → see git `+N/-N`
  update live; stage a hunk; commit; open a terminal and run a command; create a
  child workspace, commit, local-merge into parent (each strategy); trigger a
  conflict and resolve → continue; open search and replace; watch a file change
  on disk reflect live in the editor + tree + git panel.
- **Real-time correctness**: subscribe, mutate, assert the WS push lands and the
  UI updates with no lost/duplicated events; reconnect mid-session and confirm
  snapshot-on-subscribe rehydrates.
- **Performance**: use `performance_start_trace` / `performance_analyze_insight`
  and `take_heapsnapshot` to check no jank on the hot paths (typing, diff render,
  large file tree, fast file-watch bursts) and **no memory leaks** across
  open/close cycles of panes, terminals, and workspaces. Crowbar is an IDE — flag
  anything that stutters.
- **Stress**: many files changing at once (agent-style burst), many terminals,
  large repos, rapid workspace switching — confirm the debounced fan-out, the
  per-client terminal queues, and the per-repo git lock hold up.
- Capture console errors (`list_console_messages`) and failed network requests
  (`list_network_requests`) — zero unhandled errors is the bar.

## Definition of done
- All three apps run together; a real project is fully usable (no chat) — import →
  workspace → edit → git → terminal → search → live updates.
- The chrome-devtools e2e walkthroughs pass deterministically; performance traces
  show no jank on hot paths and no leaks across open/close cycles; zero unhandled
  console errors / failed requests in the happy paths.
- Frontend mock layer removed (or gated to a `dev:mock` mode only).

Report: the mock→real migration inventory, the e2e scenarios run and their
results, the performance/leak findings (with numbers), and any spec gaps the real
integration surfaced.
