# Shell spec: data-migration status (REPLACED)

Status: **REPLACED.** The GPUI shell shows a native full-screen Rust view that hides the whole layout, shows state and progress, then hands over to the normal screen. This file records what the React app does today so the replacement keeps the same information and hand-over rules. Nothing here is to be ported 1:1. Sources are `file :: symbol`, paths under `app/panes/src/` unless prefixed `api/`.

## 1. What the user sees today (there is no banner component)

There is no dedicated banner. The "small status text under the sidebar" is one of three surfaces driven by the same state:

| Surface | Where | Text | Look | Source |
|---|---|---|---|---|
| Pinned toast | bottom of the sidebar column (sidebar open) or fixed `bottom-4 left-4`, 288px wide (sidebar collapsed) | Title `Crowbar is upgrading its data`, description `Step <i> of <n>: <name>`; with no step info the title is the daemon's message (`Crowbar is starting`) and there is no description | info toast: `rounded-lg border bg-popover px-3.5 py-3 text-sm`, Phosphor `Info` icon 16px in `--info` colour, title medium, description muted, close `X`; `duration: 0` (pinned, never auto-closes), key `connection-indicator` so each step updates the same toast in place; max 3 toasts in the sidebar viewport (pinned ones are never evicted) | `components/layout/connection-indicator.tsx :: migratingToast, ConnectionIndicator`; `components/layout/sidebar-toast-overlay.tsx` |
| Landing spinner | whole content area while the first launch has no projects snapshot | `LoadingSpinner` with label = the daemon message, e.g. `Crowbar is upgrading its data (step 2 of 5: <name>)` | centred in `flex flex-1 items-center justify-center p-8`; spinner `size-4` + label `ui-text-sm` muted, `gap-1.5` | `components/layout/shell-landing.tsx :: ShellLanding` |
| Build readout line (opt-in, dev default) | sidebar foot | net state text from the same store | not user-facing in release | `components/layout/build-readout-lines.ts` (CUT per inventory) |
| Halt card (terminal failure) | replaces the whole routed app | Title `Crowbar could not start` (destructive, 14px), mono 11px message, `Quit Crowbar, fix the cause above, and open it again.` (12px muted); `Card max-w-md border-destructive/20 bg-destructive/10`, centred, 32px top drag strip | `features/window/components/daemon-halt-gate.tsx :: DaemonHaltGate` |

Toast anatomy and timings are those of part A section 10 (`sidebar.md`). Recovery toast after migration: `Reconnected` / `Backend connection restored.` (success, auto-dismiss), only shown if the migrating or disconnected toast was shown before.

## 2. Where the state comes from

Daemon side (`api/internal/core/bootgate/gate.go`, `api/internal/bootreport.go`, `api/internal/core/migrate/*`):
- Until the real API opens, the daemon serves only `GET /v0/health` and refuses everything else with HTTP 503 `{success:false, error:<message>, code:"daemon_migrating"|"daemon_halted"}`. A browser navigation gets a self-reloading HTML page (polls `/v0/health` every 1 s until `data.status === "ok"`).
- Health body: `{success:true, data:{status:"migrating"|"halted"|"ok", version, pid, detail}}`.
- `status:"migrating"`: `detail` is `null` before any step ("Crowbar is starting") or `{phase:"pre-open"|"post-open", step:{number,name}, index, total}`. Daemon message: `Crowbar is upgrading its data (step <index> of <total>: <step.name>)`.
- `status:"halted"`: `detail` = `Halt {reason, message, step?, phase?, layout, known, run?, ledger?, backup?}`; reasons: `layout_newer`, `layout_unreadable`, `ledger_corrupt`, `io_failed`, `plan_failed`, `preflight_failed`, `step_failed`, `verify_failed`, `no_implementation`. Halt is terminal until someone restarts Crowbar.
- Progress is a live `migrate.Session` read per request (`Progress()` -> Phase, Step, Index, Total). There is NO push stream while migrating: the only channel is polling the health route.

Client side (React):
- `lib/ws/manager.ts` (~line 229): each time a LOCAL-host socket fails to open (not `wasOpen`), call `probeDaemon()` (`lib/ws/daemon-probe.ts`): `GET /v0/health` once (no retries, unshared request), de-duplicated while in flight, and the answer is dropped if a socket opened since it was sent (`opened` generation counter). Each failed socket retry therefore re-polls (cadence = the websocket reconnect backoff in `lib/ws/manager.ts`, not a dedicated timer).
- Parsing: `migratingFrom(detail)`: needs numeric `index`, `total` and object `step`; name = `step.name` or `step <number>`; otherwise `{message:"Crowbar is starting"}`.
- Also any REST call refused with problem code `daemon_migrating` calls `reportDaemonRefusal` -> `reportMigrating({message})` (no step) (`lib/ws/connection-store.ts`, `lib/api/fetch.ts`); `daemon_halted` -> `useDaemonHaltStore.halt(message)`.
- Store `lib/ws/connection-store.ts`: `status` in `idle|connected|disconnected|migrating`; `migrating: {message, step?:{index,total,name}} | null`. A migrating report stands only while no local channel is open (an open socket outranks it as stale); `publish()` derives `status = migrating` when set and no socket is open.

## 3. When it shows and hides

| Condition | Shows | Hides |
|---|---|---|
| Local socket cannot open and health says migrating | toast shown immediately (no debounce, unlike the 500 ms "Backend unavailable" debounce) and updated in place each step; landing spinner if the projects scope is not synced yet | when any local channel opens (`reportChannelState(open)`) clears `migrating`; the toast is dismissed (`dismissByKey`) and, only if it had been shown, `Reconnected` is toasted |
| Crash loop (daemon crash report present) | suppressed (the crash panel speaks) | |
| Halted | toast is not used; `DaemonHaltGate` replaces the app | never, until restart |
| Health read fails | state unchanged (no new information) | |
Landing: `ShellLanding` shows the spinner only while `awaitsProjectsScope(synced)` (local daemon, projects scope not yet synced) and `migrating` is set; after the snapshot it navigates to the last/first project, `/oobe`, or `/add-host`. A snapshot that fails twice in a row (`PERSISTENT = 2`) shows `InlineError` "Could not load your projects" with Retry; one failure is treated as a restart.

## 4. Requirements for the native replacement (REPLACED view)

Intent (from the code's purpose, not its quirks):
1. While the daemon is not serving, show ONE full-screen status view (no sidebar, tabs or panes). Content: product name, a state line (`Starting` / `Upgrading your data`), step text `Step i of n: name` (hide when unknown), and a determinate progress indicator `i / n` when `total > 0` (indeterminate otherwise). Halt: the halt card text and the daemon message; the only action is Quit.
2. Source of truth is the daemon health route: poll `GET /v0/health` at a fixed short interval (the React app only polls as a side effect of socket retries; the native shell should poll explicitly, ~1 s, matching the daemon's own page) until `status:"ok"`, then open streams and hand over to the normal screen (landing logic of part 3: last project, first project, OOBE, add-host). Never show the full-screen view for a transient socket blip on an already-served session (that is the "Backend unavailable" toast, 500 ms debounce, in the normal layout).
3. Hand-over: when `ok`, fade or swap to the main layout once the projects snapshot is available (so no empty sidebar flash). The recovery "Reconnected" toast is only for outages after a successful start, not for first boot.
4. The shell supervisor (Wave 1 agent C) owns daemon start; this view reads the same health route through the shared `transport` crate. Nothing about migration state is stored by the shell.

## 5. Observed bugs and workarounds (for the "Workarounds and suspected bugs" list)
- No progress stream: progress exists only while polling happens to run, driven by socket-retry cadence; a stalled backoff means a stale step number.
- The `opened` generation counter and `inFlight` memo exist solely to discard stale health answers (race between probe and socket open).
- `migratingFrom` degrades silently to "Crowbar is starting" on any shape mismatch (including a legitimate `detail:null` at step 0), so users cannot tell "waiting to start" from "bad payload".
- Two UIs for one state (toast and landing spinner) can show at the same time on first launch.
- The migrating toast has no progress bar; the halt state is only reachable through the REST refusal path or the probe, and the toast viewport is inside the routes it replaces (comment in `daemon-halt-gate.tsx`), hence a separate gate.
