# Side-by-side parity audit: Tauri app (spec) vs GPUI shell

Date: 2026-11-18. Branch `migration/rust-shell`.

**Purpose.** This is the row list for a side-by-side capture of the Tauri desktop app on `origin/develop`, which is the spec (`desktop/src-tauri` + `web/src`), against the GPUI Rust shell on this branch. Each row is one page, flow, visible state or user-reachable interaction of the Tauri UI. The capture pass fills the empty columns. Source references are `path:line` on `origin/develop` (read with `git show origin/develop:<path>`).

**Rig.** `tools/docker/novnc-shell/README.md`. One container (`crowbar-novnc-shell`) runs both apps side by side against one seeded daemon. Drive every row with `docker exec crowbar-novnc-shell rig ...`: `rig key|type|click|scroll <app> ...`, `rig size <app> standard|min|<W>x<H>`, `rig theme light|dark`, `rig sidebar left|right`, `rig reset`, `rig pair <row-id>` (writes `~/shots/<row-id>/{tauri,rust,side-by-side}.png`), `rig perf <metric> <app>`. `mod` is `ctrl` on Linux. The seed is `crowbar-seed` (project "Crowbar Seed", repo `checkout`, workspaces `main` and `feature/pricing-rounding`, two review threads) plus `crowbar-seed-chat` (the 30-turn "Perf fixture" chat). No provider CLIs are installed, so no agent can run. Rows that need more data say `Seed: ...` in the How cell (the full list is at the end).

**Capture matrix.** Capture every row in each of these combinations unless the row says otherwise:

| Axis | Values | Rig command |
|---|---|---|
| Theme | light, dark | `rig theme light\|dark` |
| Sidebar | left, right | `rig sidebar left\|right` |
| Size | default 1280x800, minimum 640x480 | `rig size <app> standard`, `rig size <app> 640x480` (or `min`) |

Shots per row: 2 × 2 × 2 = 8 pairs. Name them `<ID>-<light|dark>-<left|right>-<1280|640>`.

**Row format.** There is one table per area, with these columns:

| Column | Filled by | Content |
|---|---|---|
| ID | this list | area prefix + number, e.g. `SB-014` |
| What | this list | the page, state or interaction |
| Tauri source | this list | `file:line` on `origin/develop` |
| How to reach/drive it | this list | keys, clicks and state the rig can reproduce; `Seed: ...` when the seeded daemon lacks the data |
| Animation | this list | duration and easing, else — |
| Shots | capture pass | shot names or a folder |
| Tauri behaviour | capture pass | what the spec does |
| Rust behaviour | capture pass | what the shell does |
| Difference | capture pass | none / the difference |
| Perf | capture pass | timing where relevant |
| Linux | capture pass | pass / fail |
| macOS | capture pass | pass / fail / n/a |

Area prefixes: OOBE, HOST, PRJ, WS, SB, VT, IDE, SPL, PANE, CHAT, TERM, BRW, ED, REV, SET, DLG, MENU, TST, DND, KEY, ANIM, WIN, A11Y, IME, UPG. Some interactions appear both in their feature area and in a cross-cutting catalogue (MENU, DND, ANIM, KEY). This is on purpose, so that each catalogue is complete.

## Contents

1. OOBE and first run (OOBE, 49 rows)
2. Hosts and pairing (HOST, 77 rows)
3. Projects (PRJ, 59 rows)
4. Workspaces (WS, 48 rows)
5. Sidebar (SB, 245 rows)
6. View tabs (VT, 53 rows)
7. IDE shell inside a pane (IDE, 74 rows)
8. Split panes (SPL, 36 rows)
9. Pane chrome (PANE, 47 rows)
10. Chat view (CHAT, 386 rows)
11. Terminal (TERM, 97 rows)
12. Browser pane (BRW, 62 rows)
13. Editor (ED, 157 rows)
14. Review and diff (REV, 88 rows)
15. Settings (SET, 132 rows)
16. Dialogs (DLG, 51 rows)
17. Menus and context menus (MENU, 125 rows)
18. Toasts (TST, 118 rows)
19. Drag and drop (DND, 100 rows)
20. Keymap (KEY, 44 rows)
21. Spinners and animations (ANIM, 107 rows)
22. Window chrome (WIN, 37 rows)
23. Accessibility (A11Y, 87 rows)
24. IME (IME, 34 rows)
25. Upgrade path (UPG, 96 rows)

- [1. OOBE and first run (OOBE)](2026-11-18-side-by-side-audit/01-oobe.md)
- [2. Hosts and pairing (HOST)](2026-11-18-side-by-side-audit/02-host.md)
- [3. Projects (PRJ)](2026-11-18-side-by-side-audit/03-prj.md)
- [4. Workspaces (WS)](2026-11-18-side-by-side-audit/04-ws.md)
- [5. Sidebar (SB)](2026-11-18-side-by-side-audit/05-sb.md)
- [6. View tabs (VT)](2026-11-18-side-by-side-audit/06-vt.md)
- [7. IDE shell inside a pane (IDE)](2026-11-18-side-by-side-audit/07-ide.md)
- [8. Split panes (SPL)](2026-11-18-side-by-side-audit/08-spl.md)
- [9. Pane chrome (PANE)](2026-11-18-side-by-side-audit/09-pane.md)
- [10. Chat view (CHAT)](2026-11-18-side-by-side-audit/10-chat.md)
- [11. Terminal (TERM)](2026-11-18-side-by-side-audit/11-term.md)
- [12. Browser pane (BRW)](2026-11-18-side-by-side-audit/12-brw.md)
- [13. Editor (ED)](2026-11-18-side-by-side-audit/13-ed.md)
- [14. Review and diff (REV)](2026-11-18-side-by-side-audit/14-rev.md)
- [15. Settings (SET)](2026-11-18-side-by-side-audit/15-set.md)
- [16. Dialogs (DLG)](2026-11-18-side-by-side-audit/16-dlg.md)
- [17. Menus and context menus (MENU)](2026-11-18-side-by-side-audit/17-menu.md)
- [18. Toasts (TST)](2026-11-18-side-by-side-audit/18-tst.md)
- [19. Drag and drop (DND)](2026-11-18-side-by-side-audit/19-dnd.md)
- [20. Keymap (KEY)](2026-11-18-side-by-side-audit/20-key.md)
- [21. Spinners and animations (ANIM)](2026-11-18-side-by-side-audit/21-anim.md)
- [22. Window chrome (WIN)](2026-11-18-side-by-side-audit/22-win.md)
- [23. Accessibility (A11Y)](2026-11-18-side-by-side-audit/23-a11y.md)
- [24. IME (IME)](2026-11-18-side-by-side-audit/24-ime.md)
- [25. Upgrade path (UPG)](2026-11-18-side-by-side-audit/25-upg.md)

## Row counts

"Seed rows" counts the rows whose How cell says `Seed:`. They need data or rig support that the current seed does not provide.

| Area | Prefix | Rows | Seed rows |
|---|---|---|---|
| OOBE and first run | OOBE | 49 | 45 |
| Hosts and pairing | HOST | 77 | 63 |
| Projects | PRJ | 59 | 34 |
| Workspaces | WS | 48 | 29 |
| Sidebar | SB | 245 | 57 |
| View tabs | VT | 53 | 11 |
| IDE shell inside a pane | IDE | 74 | 4 |
| Split panes | SPL | 36 | 2 |
| Pane chrome | PANE | 47 | 7 |
| Chat view | CHAT | 386 | 236 |
| Terminal | TERM | 97 | 11 |
| Browser pane | BRW | 62 | 16 |
| Editor | ED | 157 | 39 |
| Review and diff | REV | 88 | 19 |
| Settings | SET | 132 | 10 |
| Dialogs | DLG | 51 | 18 |
| Menus and context menus | MENU | 125 | 13 |
| Toasts | TST | 118 | 89 |
| Drag and drop | DND | 100 | 33 |
| Keymap | KEY | 44 | 5 |
| Spinners and animations | ANIM | 107 | 39 |
| Window chrome | WIN | 37 | 0 |
| Accessibility | A11Y | 87 | 41 |
| IME | IME | 34 | 34 |
| Upgrade path | UPG | 96 | 7 |
| **Total** | | **2409** | **862** |

## Seed data and rig support the current seed lacks

The current seed is `api/cmd/crowbar-seed` plus `api/cmd/crowbar-seed-chat`. It provides one project, one repo, two workspaces with a clean working tree, two review threads and one finished 30-turn chat. It installs no provider. Some rows need more:

**Providers and running agents**
- An enabled provider CLI that can start a session, including on a terminal (PTY). It needs a model catalogue, effort levels, permission levels, compaction, hotswap and a slash catalogue with commands, terminal-only commands and namespaced skills. Add a second enabled provider for cycle-provider, provider reorder and the provider-switch list.
- A turn in progress in each of these states:
  - streaming text
  - live reasoning
  - plain working
  - "Stopping…"
  - compacting
  - a provider switch in flight
  - a pending move ("Moving to … after this turn")
  - a running workspace operation
- A running agent session, for Active sessions and Kill, "The agent is busy", "Can't … while the chat is working", taking a prompt back and Stop and merge.
- An agent or MCP client driving the `browser_*` tools, for the "Agent" badge and tabs the agent opens.

**Chat ledger contents**
- Tool calls by status: running, error (with a body), declined, interrupted, abandoned, awaiting an ask. Also read calls with `kind=read`: the seeded `read_file` calls carry no kind, so they do not fold into "Read 5 files".
- Tool calls by kind:
  - edit with a unified diff, a new-file diff, a huge diff, an unparseable diff and a live diff stream
  - execute with output
  - search, fetch, subagent, MCP, `mcp__crowbar__` and an unknown kind
  - payload refs, including a ref that fails to load and one whose blob is gone
  - a mixed read/search/fetch run, and a run with a failure in the middle
- Asks:
  - permissions for execute, edit, MCP, plan and an unknown family
  - single, multi and three-question asks, and three asks pending at once
  - a terminal-only ask, an ask with no relay, and slow or failing answer endpoints
  - elicitation forms
  - resolved asks: allowed, allowed with a grant, denied, timed out, not answered
- Outbox: sending, pending, queued (one and three), and prompts refused or failed with network, session_lost, prompt_submit_unsupported and too_large.
- Markers:
  - turn_failed (auth, limit with reset time, overloaded, other)
  - session_lost, runner_exited with spawn_failed
  - turn_interrupted variants
  - compaction, provider, model and effort changes
  - move and branch-out markers in every phase
  - an unknown marker
- Notices: warning, context warning, session_conflict, legacy_unreadable.
- Subagents running, done, failed, interrupted and abandoned. Background shells in every state (six at once), and a running monitor.
- Reasoning blocks with timings, a plan snapshot, todo trays, telemetry (context at 40% and at 90%, tokens, rate-limit windows, cost).
- Message content:
  - long, multi-line and markdown user messages
  - a settled `/command`
  - harness and note messages
  - replies with headings, lists, fenced code (including a late-loading language), links, tables, blockquote/hr, callouts, images, file and pasted-text attachments, mermaid (streaming, settled, open fence) and excalidraw (with and without PNG)
  - a message whose renderer throws
- A chat longer than one feed window (older-turns paging), an untitled chat, a nested thread, and a plain repo thread with no worktree. Add a chat in the `main` workspace for "Make a branch".

**Projects, repos, workspaces**
- A daemon with no projects (OOBE, "Add a space"), and one with no git on PATH. gh and glab installed, signed in and signed out.
- A second project with chats (switching, ctrl+shift+1…9, carousel, cross-space toasts), a project with no repos, a second repo in "Crowbar Seed", and a spare git folder plus an image file to import.
- Sidebar folders (in the repo and in the project home) and enough rows (about 40) to overflow the tree.
- Workspace states:
  - locked
  - placeholder or failed provision
  - never opened
  - a branch held by a foreign worktree
  - a generated `chat-xxxxxxxx` branch
  - over 999 changed lines
  - a git status that fails
  - merged ("Merged into")
  - conflicting with its parent, a protected parent, no parent
  - threads resolved (merge allowed)
  - a removal the planner refuses
  - a delete that cascades to sibling chats
  - a branch that fails to provision
- PR states: open, merged, closed, deleted, conflicts. A reachable git remote with protected, already-imported and new branches.
- Uncommitted changes in the feature workspace: modified, staged, untracked, deleted, renamed, including files under `src/`.

**Files and diffs**
- Checkout contents:
  - a `.gitignore` with an ignored file
  - a dotfile and a symlink
  - `.env` and `.env.example`
  - a nested folder with a single-child chain
  - a folder over 100 files
  - a deep, long path
  - a long TS file, a `.html`, a binary file and an image
  - a file over the size limit and a chmod 000 file
  - a Crowbar state-file path
  - a README with a table, fenced code, anchor and relative links
  - a `.md` with frontmatter and images
  - markdown that crashes Plate
- Branch diff: a deleted, renamed and binary file, a changed image, many files, a minified or huge file, a hunk over the line cap, hidden context between hunks, more than 200 search hits, an empty commit, and a review description.
- Review threads: one by the current identity, one agent-authored with a chatId, one with a reply, one outdated or off-patch, and one posted while the review is open.
- A TypeScript language server with node_modules, for hover, completion, diagnostics, definition, references, rename, code actions, format and Restart. Also a server with code lens and one that errors.

**Hosts and pairing**
- A reachable remote daemon running `crowbar pair` (valid and wrong codes, rate limit, refusal, certificate change), and a non-Crowbar listener.
- Paired hosts in each state: online, offline, 401/needs pairing, identity changed, no stream, never probed, removed, 5xx, failing `/browse`, empty root, revoked on the host. Add two paired hosts and one host reachable by two addresses.
- A Secret Service keyring that works, and one that is missing or locked.

**Daemon and failure injection**
- Daemon states: stopped and restarted mid-session, killed once (respawn), crash-looping (four deaths, plus one failed restart), wedged (watchdog), migrating with steps, halted, failing `crowbar-api paths`, a projects stream that closes before its snapshot, a slow snapshot.
- Refusals for:
  - rename, icon, lock, move, remove, delete, git, strategy changes
  - comment POST, patch fetch
  - PTY create, upload, compact, permission-level writes
- Forced client failures:
  - a render error (error boundary, pane Retry)
  - editor chunk or Monaco load
  - transcript render
  - IndexedDB write, clipboard read and write
  - native popup
  - a document deleted while open
  - a failed boot or session restore

**Settings and persistence**
- An exported `crowbar-settings.json`, an invalid JSON file and a valid agent descriptor YAML on disk.
- A dev build of the Tauri app (Developer tab, build readout), or the build readout turned on.
- Legacy storage from older builds: a `workspace:<id>:state` localStorage key, a v13 IndexedDB, a layout row older than v4, a `crowbar-parser-cache` DB, and a persisted pinned view.
- Optionally, an installed Nerd Font.

**Rig additions** (not seed data, but rows need them)
- A drag primitive (press, move, release) in `rig`.
- An X file manager or `dragon`, plus files on disk (one with a space in its name, a .txt, a file over 25 MB), for OS drops into chat, editor, terminal and file tree.
- Clipboard fixtures: an image, multi-line text over 6 lines, and a file copied from a file manager.
- A local HTTP server for the browser pane, with links, `target=_blank`, a form, a long page, a changing `<title>`, a redirect, a download and a slow endpoint.
- A way to restart, stop and start the daemon from `rig`.
- ibus or fcitx with a CJK engine, an xkb layout with dead keys, and a Compose key, for the IME rows.
- An AT-SPI inspector (accerciser) and Orca or an AT-SPI event monitor, for the A11Y rows.
- A reduced-motion switch (GTK `gtk-enable-animations=false`).
- At least 10 open tabs and 5 or 6 views, for overflow. Two panes in a split, and a third chat for nested splits.
- A macOS host for the macOS-only rows: native app menu, traffic lights, vibrancy, New Window, Cmd+Q, fullscreen, the About panel, Clear browsing data.
