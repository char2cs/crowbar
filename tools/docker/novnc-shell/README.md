# crowbar-novnc-shell: the Tauri app and the Rust shell side by side

One container, one noVNC desktop (port 6090 on the host, no password, any Host header), one daemon
and one instance of each app:

- **Tauri** — the desktop app built from `origin/develop` (`desktop/src-tauri` + `web/`), left half.
- **Rust** — the GPUI shell built from this branch (`app/desktop/app`, Release), right half.
- **Daemon** — `api/cmd/crowbar` from this branch, one `CROWBAR_HOME=/home/user/.crowbar-rig`, one
  socket. Its home comes from a seed profile (see Seed profiles): `baseline` unless the container
  is started with `RIG_PROFILE=empty|full`.

Both windows are 1280x800 (`RIG_W`/`RIG_H`), undecorated, side by side on a 2600x900 Xvfb screen, with
the same theme (dark by default) and the sidebar on the left. The container has no host mounts, no
docker socket, no host network and no credentials. No real agent CLI is installed: chats run the
**scripted provider**, a deterministic stand-in (see Scripted provider), and pull-request state
comes from a stand-in forge CLI (see Forge stand-in).

## How both apps share one daemon

Each app wants to spawn its own daemon (`crowbar-api serve`) and talk to the socket that
`crowbar-api paths` reports. Both are pointed at `/opt/rig/bin/crowbar-api` (Tauri's sidecar
`/usr/bin/crowbar-api` is a symlink to it; the Rust shell gets `CROWBAR_API_BIN`). For the app named
by `RIG_APP`, that wrapper reports `~/.rig/<app>/daemon.sock` and, for `serve`, execs
`rig-proxy.py`: a socket proxy to the real daemon that splices every request through unchanged
and rewrites only the `pid` in `/v0/health` replies to its own (the Rust shell refuses a daemon
whose pid is not the child it spawned). The real daemon is run by the entrypoint (`rig
_daemon-loop`), so stopping or restarting an app never touches it. Both apps go through the same
proxy, so its cost (one Python hop) is identical for both; perf commands exclude it from the
process trees they measure.

## Build

    tools/docker/novnc-shell/build.sh                 # compile everything, then build the image
    tools/docker/novnc-shell/build.sh --stage <dir>   # image only, from already-built artefacts

The image build ends with `seed/build.sh`, which runs the daemon once per profile and saves each as
`/opt/rig/profiles/<name>.tar` (about 1 minute; see Seed profiles).

`build.sh` exports `origin/develop` (`desktop`, `web`) and `HEAD` (`api`, `app/panes`) with `git
archive` into a temp dir — nothing is checked out in the worktree — then, in a throwaway
`desksplit-rust` container, builds the daemon and both seeders (`-tags noEmbed`), the panes
(`bun run build:panes`), the develop web and the Tauri `.deb` (`@tauri-apps/cli@2.12.4`, the
daemon from `HEAD` as its sidecar). The Rust shell is built Release in the shared `crowbar-build`
container (`tools/scripts/rbuild.sh`, target dir `/tmp/rig-target` in that container). The temp
dir, its caches and `/tmp/rig-target` are deleted when it exits. `--stage <dir>` expects
`crowbar`, `crowbar-api`, `crowbar-seed`, `crowbar-seed-chat`, `panes/` and `crowbar-desktop.deb`.

## Run and open

    tools/docker/novnc-shell/run.sh       # (re)creates the container crowbar-novnc-shell

Open `http://<host>:6090/` (redirects to `vnc.html?autoconnect=true&resize=scale`). On first boot the
entrypoint extracts the `RIG_PROFILE` profile (default `baseline`) and runs `rig init` (about 20 s);
later boots run `rig start all`. Every boot starts the web fixture (`rig http start`). Everything below runs
inside the container: `docker exec crowbar-novnc-shell rig <command>`. Copy screenshots and perf
results out with `docker cp crowbar-novnc-shell:/home/user/shots .` and
`docker cp crowbar-novnc-shell:/home/user/.rig/perf .`.

Logs: `~/.rig/daemon.log` (the shared daemon), `~/.rig/http.log` (web fixture), `~/.rig/ibus.log`, `~/.rig/<app>/app.log`, `~/.rig/<app>/daemon.log`
(what each app's supervisor saw), `~/.rig/entrypoint.log`, `$CROWBAR_HOME/shell/logs/crowbar.log`.

## Driver

`<app>` is `tauri` or `rust`; `[app]` also takes `all` (the default). Points are window-relative pixels.

    rig status                          # pids, window ids and geometry, daemon health
    rig start|stop|restart [app]        # exactly one instance each; start waits for the window and places it
    rig init                            # same theme + sidebar in both; on baseline, both seeded chats open as tabs
    rig seed [list]                     # the current profile and the ones built into the image
    rig seed empty|baseline|full        # apps stopped, daemon paused, that profile restored, apps' storage wiped, init
    rig reset                           # rig seed <current profile>
    rig daemon status|stop|start|restart|kill   # the one shared daemon: stop holds it down until start;
                                        # restart and kill let the daemon loop bring it straight back
    rig daemon wedge|unwedge            # SIGSTOP/SIGCONT: the socket accepts, nothing answers
    rig daemon crashloop on|off         # the loop SIGKILLs every start after about 2 s
    rig size [app] standard             # 1280x800 (RIG_WxRIG_H): the size every comparison uses
    rig size [app] default              # each app's own default: Tauri 1200x800, Rust 1280x800
    rig size [app] min                  # the minimum the app declares (WM_NORMAL_HINTS), else RIG_MIN
    rig size [app] <W>x<H>              # any exact client size
    rig theme light|dark|system [app]   # through each app's own settings store, restarting it
    rig sidebar left|right [app]        # same
    rig key <app> <keys...>             # xdotool key names, e.g. ctrl+t Escape; sent with XTest
    rig type <app> <text>
    rig click <app> <x> <y> [button]
    rig scroll <app> <x> <y> [up|down] [clicks]
    rig drag <app> <x1> <y1> <x2> <y2> [steps] [button] [hold-ms]   # press, move in steps (20), release
    rig shot <app> [file.png]           # one window to PNG (default ~/shots/<app>-<time>.png)
    rig pair <row-id>                   # ~/shots/<row-id>/{tauri,rust,side-by-side}.png
    rig perf <metric|all> [app] [opts]  # see Performance
    rig winid <app>

Where each app keeps the two settings (the apps are stopped while the rig writes them):

- Tauri: WebKit localStorage of the `tauri://localhost` origin,
  `~/.local/share/software.rabbyte.crowbar/localstorage/tauri_localhost_0.localstorage` (SQLite,
  table `ItemTable`, UTF-16LE values), keys `crowbar:settings:themeMode` and
  `crowbar:settings:sidebarPosition` (`web/src/features/settings/lib/settings-persistence.ts`).
  WebKit discards a database it did not create, so if it is missing the rig starts Tauri once first.
- Rust: `$CROWBAR_HOME/shell/settings.json`, keys `themeMode` and `sidebarPosition`
  (`shell_view/appearance_settings.rs`, `shell_view/sidebar_position.rs`).

Environment and input (each setting that the apps read at start restarts them):

    rig clip set <text>                 # CLIPBOARD as text (xclip keeps owning it until replaced)
    rig clip file <path> [mime]         # a file's bytes under its MIME type (guessed from the name)
    rig clip copy-file <path>           # a file "copied" in a file manager (x-special/gnome-copied-files)
    rig clip uri <path>                 # text/uri-list
    rig clip lines|image                # ~/fixtures/clipboard/lines.txt (12 lines) or image.png
    rig clip get [target] | targets | clear
    rig http start|stop|status|url [path]   # web fixture on 127.0.0.1:8088 (RIG_HTTP_PORT), see below
    rig motion reduced|full [app]       # gtk-enable-animations in ~/.config/gtk-3.0/settings.ini
    rig ime on [engine]|off|engine <e>|engines|status   # ibus (libpinyin by default) on a session bus
    rig keys compose|deadkeys|reset|show    # setxkbmap: Compose on Right Alt; `deadkeys` = us intl
    rig a11y on|off|status              # AT-SPI registry on a session bus, the apps' bridge on
    rig a11y dump <app> [--depth N]     # the app's accessibility tree (role, name, extents, states)
    rig a11y apps                       # applications registered on the AT-SPI bus

`rig ime`, `rig a11y` and `rig motion` leave the apps' extra environment in `~/.rig/app.env`
(`NAME=value` or `-u NAME` lines) and the session bus address in `~/.rig/bus.env`.

- **Reduced motion.** WebKitGTK maps `gtk-enable-animations=false` to `prefers-reduced-motion:
  reduce`, so it reaches Tauri and the panes webview inside the Rust shell. GPUI has no OS motion
  input on Linux: the shell's own animations (`app/desktop/crates/ui/src/motion.rs`) ignore it.
- **IME.** `rig ime on` starts `ibus-daemon --xim` with the pinyin engine and restarts the apps with
  `GTK_IM_MODULE=ibus XMODIFIERS=@im=ibus`. Typing latin letters into a focused field then opens the
  candidate window; `rig key <app> space` commits the first candidate. `rig ime engine xkb:us::eng`
  switches to plain latin without leaving ibus.
- **Accessibility.** The image sets `NO_AT_BRIDGE=1`; `rig a11y on` unsets it for the apps and sets
  `GTK_A11Y=atspi`. `a11y-dump.py` (pyatspi) prints what each app exposes.

Daemon data and chats (all through the daemon's HTTP API, over its socket):

    rig api <METHOD> <path> [json|@file]    # one call; prints `data` (e.g. rig api GET /v0/projects)
    rig ledger <chat-id>                # the chat's ledger rows, one JSON object per line
    rig agent scenarios                 # the scripted provider's scenarios
    rig agent chats                     # every chat: id, project/repo, workspace, title
    rig agent new [--home] [--project NAME] [--workspace <ws>] [--title T] [--parent <chat>] [--provider P]
    rig agent play <chat> <scenario> [args]   # sends "#<scenario> args", waits for the turn (RIG_TIMEOUT, 60 s)
    rig agent say <chat> <text>         # a plain prompt (the reply scenario)
    rig agent asks <chat>               # pending asks and their option ids
    rig agent answer <chat> <option-id[,id]> [ask-id]
    rig agent stop <chat>
    rig provider list|enable <id>|disable <id>
    rig forge signed-in|signed-out|status
    rig remote start|stop|restart|status    # a second daemon as a paired remote host (see Remote host)
    rig remote code                     # a one-time pairing code, the fingerprint and the addresses
    rig remote clients|revoke           # paired clients; revoke them all (the app then gets 401)
    rig remote rekey                    # new TLS key: the pinned fingerprint stops matching
    rig remote providers on|off         # every provider on the remote disabled or not
    rig remote api <METHOD> <path> [json]   # the remote daemon's API, over its own socket
    rig remote wipe                     # its home deleted; the next start seeds it again

`points.conf` (copy to `~/.rig/points.conf` to override) names the points the rig clicks: the two
seeded chats in the sidebar, tab 1 / tab 2 in the tab strip, the chat composer and the transcript.
They are positions in the **baseline** profile's sidebar; `rig perf` assumes baseline.

## Seed profiles

`seed/build.sh` builds three profiles at image build time, each a tar of `~/.crowbar-rig` (the
daemon home), `~/.scripted-agent` (the scripted CLI's own state) and `~/fixtures`. `rig seed <name>`
restores one exactly (and stops and wipes the remote host), so a capture that mutates data is undone by `rig reset`. Files that must stay
unreadable are archived readable and locked again on restore (`~/.rig-unreadable` lists them).

- **empty** — a fresh daemon: no projects, the two scripted descriptors installed, providers in
  their default order. For first-run and empty states.
- **baseline** — what the rig always had: `crowbar-seed` (project "Crowbar Seed", repo `checkout`,
  workspaces `main` and `feature/pricing-rounding`, two review threads) and `crowbar-seed-chat` (the
  30-turn "Perf fixture" chat). `points.conf` and `rig perf` assume it.
- **full** — baseline, then `seed/full.py` drives the API and the scripted provider (about 1 min).
  It writes a summary to `~/.rig/seed-full.txt` (chat and workspace ids):
  - **Remote.** `~/fixtures/remote/gitlab-fixture/checkout.git` is `origin`, with `main`, the feature
    branch, `remote-only/new-feature` (not checked out anywhere) and `release/1.0`.
  - **Checkout with every file kind** (`feature/review-everything` worktree, `seed/files.py`):
    dotfiles, a gitignored `.env` and `build/`, markdown with frontmatter, images, tables and links,
    a 1500-line module, html, png, binary, a single-child folder chain, a deep long path, a
    120-file folder, symlinks (in-tree and to the Crowbar state dir), an executable, a sparse file
    over the 25 MiB read cap and a mode-000 file.
  - **Branch diff** on that branch against main: a deleted file, a rename, an image and a binary
    change, two-hunk edits, a minified bundle, a 25000-line file (over the 20000-line patch cap),
    60 changed files, 250 matches of one needle, and an empty last commit.
  - **Uncommitted changes** in `feature/pricing-rounding`: modified, staged, deleted, renamed,
    untracked.
  - **Workspaces**: locked, a generated `chat-*` branch, over 999 changed lines, git status
    failing (broken `.git`), conflicting with main, already merged into main, PR open, PR merged,
    PR closed (from the forge stand-in), every thread resolved, adopted from a pushed branch with
    no parent, a placeholder for a branch held by a foreign worktree, and one whose chat has three
    sibling chats (delete cascades).
  - **Review threads**: by the user, with a reply, agent-authored, outdated (the line moved),
    resolved, and one posted by the agent over Crowbar's MCP server (carries a `chatId`).
  - **Chat ledgers** (in `feature/review-everything`), each played with the scripted provider:
    `Ledger: tools` (every tool kind and end state), `Ledger: asks` (allowed, granted for the
    session, denied, answered questions, a filled form, one left pending), `Ledger: markers`
    (failures auth/limit/overloaded/other/none, compaction, model, effort and provider changes,
    session lost, runner exited, interrupted), `Ledger: notices` (warnings, alert, telemetry),
    `Ledger: agents` (subagents, shells, plan), `Ledger: rich` (markdown, mermaid, long and
    multi-line user messages, a slash command), a 120-turn chat, an untitled chat, a nested thread,
    a plain repo thread and a chat on main.
  - **Sidebar**: folders `Experiments` and `Archive` (nested) and 24 filler chats in the repo.
  - **Projects**: "Crowbar Seed" (+ repo `second-repo` on `develop`, a home chat, a home folder
    `Notes` with two notes), "Side Project" (repo `side-app`, two chats), "Empty Space" (no repos).
  - **Fixtures** in `~/fixtures`: `drop/` (files to drag in, one with spaces, a script, a 26 MB
    sparse file), `clipboard/`, `import/` (a spare git repo to import, an icon), `settings/` (an
    export, an invalid json, a provider descriptor to upload), `forge/`.

Live states (a turn running, an ask pending, a chat compacting or stopping) do not survive a
restore: the daemon ends a runner on restart. Make them on demand, e.g. `rig agent play <chat>
working` in one shell and capture; `rig agent stop <chat>` ends it. Telemetry is held in memory
too, so `#telemetry-40` / `#telemetry-90` have to be played after the restore.

## Scripted provider

`agent/scripted.yaml` (id `scripted`) and `agent/scripted-lite.yaml` (id `scripted-lite`: chat
only, two models, no effort, no slash catalog) are ordinary provider descriptors, installed into
every profile through `POST /v0/settings/chat/descriptors`. They spawn `scripted-agent`
(`/usr/local/bin`, Python stdlib) over the hooks transport: the daemon starts it in a PTY with the
prompt as argv, and it posts canonical hook payloads (`message_delta`, `tool_pre`, `permission`,
`turn_stop`, ...) to the daemon. It reports three models with efforts (`scripted-agent models
--json`), a plugin catalog, and the permission levels guarded, trusted and full-auto. It receives
Crowbar's MCP config like a real CLI and can call Crowbar's MCP tools.

A prompt `#<scenario> args` plays `/opt/rig/agent/scenarios/<scenario>.json`; any other prompt
plays `reply` (reasoning, the prompt echoed back as markdown, telemetry); `/compact` plays
`compact`. After the turn the CLI stays up as a line-mode TUI in the chat's terminal: each line
typed there is another prompt. `rig agent scenarios` lists them all; the families are replies and
markdown, tool calls of every kind and status, edits and diffs, permission asks, questions and
forms, subagents, shells, plans, telemetry, warnings and alerts, failures of every kind,
compaction, a session lost on resume, a CLI that exits mid-turn, a slow stop, turns that hold
until stopped (working, streaming, reasoning, tool running, compacting), and what only the
terminal shows: a workspace-trust modal Crowbar can answer (`trust`), a modal it cannot
(`blocked`, answered by Enter in the terminal) and a usage-limit banner that ends the turn
(`usage-limit`), matched by the descriptor's `terminal_prompts` / `terminal_notices`.

A scenario is `{"description", "steps": [...]}`; each step is one of `text`, `echo`, `reason`,
`tool`, `ask`, `elicit`, `subagent`, `plan`, `plan-step`, `telemetry`, `diff`, `compact`, `post`
(any raw event), `screen` (PTY text only, optionally waiting for Enter), `repeat`, `sleep`,
`hang`, `stop`, `fail`, `exit`, `slow-exit`, `lose-session`, `mcp` (see `Turn.do_*` in `agent/scripted-agent`). Drop a new JSON file into the scenarios
directory to add one. `SCRIPTED_SPEED` scales every delay (`0` plays instantly; the seed uses it).

## Forge stand-in

The daemon reads pull-request state through the forge's CLI, chosen from the repo's `origin` URL
(`gh` for github, `glab` for gitlab), and only while a client holds the workspace's `forge:` scope.
`forge/glab` is installed as `/usr/local/bin/glab`; the full profile's remote has `gitlab` in its
path, so the daemon asks it. It answers `auth status`, `mr list` and the protected-branches call
from `~/fixtures/forge/merge-requests.json`. `rig forge signed-out` makes `auth status` fail (forge
present but signed out); edit the json to change a merge request's state.

## Language server

The daemon starts `typescript-language-server --stdio` for TypeScript files when it is on PATH. The
image carries it (`typescript-language-server@4.3.3` with `typescript@5.6.3`, on Ubuntu's
`nodejs`), so hover, diagnostics, go-to-definition and semantic tokens work in the seeded repos'
`.ts` files. No other language server is installed.

## Remote host

`rig remote start` runs a second daemon in the container with its own home (`~/.crowbar-remote`)
listening on `tcp://:4747` (`RIG_REMOTE_PORT`). A non-loopback bind makes the daemon terminate TLS
and accept only paired clients, exactly as a remote machine does; it answers on `127.0.0.1:4747`,
`localhost:4747` and the container's own address. Its first start seeds it (`seed/remote.py`: the
scripted descriptors, project "Remote Project" with repo `remote-app` and one chat). Pair from
either app with the code and fingerprint `rig remote code` prints. Host states: down (`stop`),
answering 401 (`revoke`), re-keyed (`rekey`), without providers (`providers off`), reachable by
two addresses (pair once by `127.0.0.1`, once by `localhost`). `rig seed` stops and wipes it.

Tauri keeps host tokens in the OS keyring (`keyring` crate, kernel keyutils + Secret Service). The
docker default seccomp profile refuses `keyctl`, so in this container there is no working keyring:
that is the "no keyring" state, and a "working keyring" capture needs the container run with a
seccomp profile that allows `keyctl` plus a Secret Service (not in the image).

## Web fixture

`rig/fixture-http.py`, on `127.0.0.1:8088`: `/` (links, a `target=_blank` link, an anchor, an
image), `/long` (60 sections), `/form` (POST echoes the fields), `/title` (title changes every
second), `/redirect` (302 to `/`), `/download` (an attachment), `/slow?ms=N`, `/logo.png`, and a
404 for anything else.

## Performance

Every metric runs one app at a time: the other app is stopped for the measurement and started
again afterwards (`RIG_KEEP_OTHER=1` keeps it). Each prints one JSON line and appends it to
`~/.rig/perf/results.jsonl`.

    rig perf startup [app]                       # --quiet 5000 --settle-px 512 --limit 120
    rig perf input [app]                         # --n 30 --key m --gap 250
    rig perf switch [app]                        # --n 10 --quiet 1500
    rig perf scroll [app]                        # --rate 60 --seconds 2
    rig perf idle [app]                          # --seconds 60
    rig perf memory [app]                        # --tabs 5 --key ctrl+j --settle 3
    rig perf all [app]

**Frames.** The sampler (`rigx.py`) watches X Damage on the root window; on every damage event it
reads the app's client rectangle from the root window (Xvfb has no compositor and the windows never
overlap, so that is exactly what is on screen) and compares it with the previous frame. A
*changed frame* differs in at least `--min-px` pixels (24 by default; a blinking caret is below it,
and GTK cursor blink is off for both apps). A frame is timed when its damage event arrives, before
the read. Reading and comparing a 1280x800 frame takes about 8 ms, which bounds the resolution.
Keys and clicks are injected with XTest from the same process, timed just before the request is sent.

- **startup**: from `exec` (the `rig start` call) to *interactive*: the first changed frame of the
  window rectangle that is within `--settle-px` (512) pixels of the frame the window settles on, where
  settled means no changed frame for `--quiet` (5 s). Both apps restore the seeded workspace with the
  "Perf fixture" chat tab active, so the settled frame is the fully drawn transcript; it is saved as
  `~/.rig/perf/<app>-startup-final.png` to check. `first_paint_ms` is the first changed frame at all.
- **input**: click the composer of the Perf fixture tab, then N times: wait for 250 ms without a
  changed frame, press and release `m`, and time press -> first changed frame. Median and p95. The
  typed characters are erased afterwards.
- **switch**: alternate clicks on tab 1 and tab 2 of the tab strip, N times. `first_change_ms` is
  click -> first changed frame; `settled_ms` is click -> the last changed frame before 1.5 s of
  quiet. The pointer is moved onto the target and hover effects settle before each timed click.
- **scroll**: with the pointer over the transcript (opened scrolled to its end), send wheel-up clicks
  at 60 Hz for 2 s, then record every changed frame until 1 s after the last click. Reports changed
  frames, fps over the span, frame intervals (median, p95, max) and how many exceed 33.4 ms. A
  second, unmeasured pass scrolls back down.
- **idle**: CPU time (utime + stime from `/proc/<pid>/stat`) of the whole process tree — the app and
  every descendant (WebKitWebProcess, WebKitNetworkProcess, ...), minus the rig's socket proxy — over
  60 s with no input, as a percentage of one core. The shared daemon is not part of either tree.
- **memory**: restart the app, wait for the startup quiet period (5 s) plus 3 s, sum `Pss` from
  `/proc/<pid>/smaps_rollup` over the same process tree. Then 5 times: click tab 1 (so the key does
  not land in a terminal), press `ctrl+j` (New terminal tab, the same command in both apps), wait for
  1.5 s without a change (1000-pixel threshold: a terminal cursor blinks) plus 3 s, sum PSS again.
  `per_tab_mib` is the mean increase. Both apps persist the new tabs and a terminal swallows `ctrl+w`,
  so the metric ends with `rig reset` (`RIG_NO_RESET=1` skips it).
