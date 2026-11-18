# crowbar-novnc-shell: the Tauri app and the Rust shell side by side

One container, one noVNC desktop (port 6090 on the host, no password, any Host header), one daemon
and one instance of each app:

- **Tauri** — the desktop app built from `origin/develop` (`desktop/src-tauri` + `web/`), left half.
- **Rust** — the GPUI shell built from this branch (`app/desktop/app`, Release), right half.
- **Daemon** — `api/cmd/crowbar` from this branch, one `CROWBAR_HOME=/home/user/.crowbar-rig`, one
  socket. It is seeded at image build time with `crowbar-seed` (project "Crowbar Seed", repo
  `checkout`, workspaces `main` and `feature/pricing-rounding`, two review threads) and
  `crowbar-seed-chat` (the 30-turn "Perf fixture" chat in the feature workspace).

Both windows are 1280x800 (`RIG_W`/`RIG_H`), undecorated, side by side on a 2600x900 Xvfb screen, with
the same theme (dark by default) and the sidebar on the left. The container has no host mounts, no
docker socket, no host network and no credentials; the provider CLIs are not installed, so chats
cannot run an agent.

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
entrypoint runs `rig init` (about 20 s); later boots run `rig start all`. Everything below runs
inside the container: `docker exec crowbar-novnc-shell rig <command>`. Copy screenshots and perf
results out with `docker cp crowbar-novnc-shell:/home/user/shots .` and
`docker cp crowbar-novnc-shell:/home/user/.rig/perf .`.

Logs: `~/.rig/daemon.log` (the shared daemon), `~/.rig/<app>/app.log`, `~/.rig/<app>/daemon.log`
(what each app's supervisor saw), `~/.rig/entrypoint.log`, `$CROWBAR_HOME/shell/logs/crowbar.log`.

## Driver

`<app>` is `tauri` or `rust`; `[app]` also takes `all` (the default). Points are window-relative pixels.

    rig status                          # pids, window ids and geometry, daemon health
    rig start|stop|restart [app]        # exactly one instance each; start waits for the window and places it
    rig init                            # same theme + sidebar in both, both seeded chats open as tabs
    rig reset                           # seeded daemon home back, both apps' storage wiped, then init
    rig daemon status|restart           # the one shared daemon
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

`points.conf` (copy to `~/.rig/points.conf` to override) names the points the rig clicks: the two
seeded chats in the sidebar, tab 1 / tab 2 in the tab strip, the chat composer and the transcript.

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
