# End-to-end suite

Drives a real dev instance of THIS checkout and asserts through the DOM and
the daemon. One command:

    make e2e            # the Tauri desktop app (debug build), the real thing
    make e2e-browser    # the same web bundle in a headless Chrome, faster
    make e2e ARGS=--only=console,recents    # a subset
    make e2e ARGS=--keep                    # keep the run dir on success too

Needs bun, Go, Rust (for `e2e`) and Chrome (for `e2e-browser`; override the
binary with `CROWBAR_E2E_CHROME`). Nothing to log in to.

## What a run does

1. Creates an isolated run dir under the OS temp dir: its own `CROWBAR_HOME`
   (daemon, socket, projects, logs) and its own Vite origin on a free port, so
   webview storage is fresh too. It refuses to run against `~/.crowbar` and
   never touches it or `/Applications/Crowbar.app`.
2. Installs a stub provider (`lib/stub-provider.mjs`: `cat` on a PTY behind a
   uniquely named symlink, with hook mappings) so no real CLI login is needed.
   Scenarios play provider hooks at the daemon exactly as the in-PTY hook
   command does.
3. Builds the daemon, starts Vite and the app, seeds a throwaway git repo and
   project through the daemon API, then runs the scenarios in order.
4. On failure: a screenshot per scenario in `scripts/e2e/artifacts/` (gitignored)
   and the run dir (daemon, Vite and app logs) is kept and printed. On success
   everything is removed.

All waits poll a real condition against a deadline (`lib/wait.mjs`); there are
no sleeps.

## Targets

- `tauri`: `tauri dev --no-watch` with the app's debug-only bridge
  (tauri-plugin-mcp-bridge, the WebSocket the Tauri MCP uses, port 9223+). The
  runner finds its own app by the page origin. Page scripts must be synchronous
  (the bridge evaluates natively on WKWebView); `lib/driver.mjs` enforces it.
  The daemon is the app's own sidecar. "Relaunch" restarts the whole app
  process; "daemon restart" kills the sidecar and waits for the supervisor.
- `browser`: Chrome over CDP (`lib/cdp.mjs`) plus a daemon the suite starts on
  loopback TCP. Not WKWebView, and "relaunch" is a new tab in the same browser
  (a killed Chrome loses its last localStorage writes), so full-process
  persistence is only proven on `tauri`. The native menu event is skipped.

## Scenarios (`scenarios/`, one per feature area)

| file             | covers                                                                                                                                                                                                                   |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| console          | banner open/close, Escape, outside press (overlay closes, push stays and shrinks the app), Mod+\` and the `console:toggle` menu event, live "turn started" line, "Daemon restarted" note, dock/mode/size across relaunch |
| recents          | 9 views, 27 rapid switches: zero mounts/unmounts of workspace slot, view root, chat view and pane container; time-to-visible                                                                                             |
| close-view       | closing a view retires its provider process and leaves the chat dormant; other chats keep theirs                                                                                                                         |
| subagent         | hooks carrying an agent id draw no rows; a hand-back envelope is a harness row, not a user bubble                                                                                                                        |
| codex-order      | text, tool, more text keeps the bubble above the tool row                                                                                                                                                                |
| working-line     | heading shows the reasoning headline, else a verb; heading face and weight                                                                                                                                               |
| workspace-switch | focus moves across 4 warm workspaces issue no files/git/review/threads fetches (with a positive control)                                                                                                                 |
| sidebar-band     | nightly band, light and dark                                                                                                                                                                                             |

## Not covered here (needs a human, or is a known limit)

- A physical Cmd+\` on macOS (the system and the native menu own it; only the
  DOM chord and the menu event are driven).
- A real Claude or Codex: the stub proves Crowbar's handling of the hook
  stream, not what a provider emits.
- Tiled panes via drag-and-drop: workspace-switch switches between Recents rows
  (also cross-workspace focus), because a synthetic drag is not reliable.
- The same chat shown in two panes (closing one view must keep the runner):
  there is no UI path to it; the unit tests cover it.
- Visual fidelity of fonts and the band artwork beyond computed styles.

## Adding a scenario

Create `scenarios/<area>.mjs` exporting `{ name, run(ctx) }`, register it in
`run.mjs`. `ctx` has `d` (page driver: `eval`, `until`, `emit`), `daemon`, `fx`
(project/repo ids), `inst` and `target`. Select by stable `data-*`
attributes and roles, never by class names or structure.
