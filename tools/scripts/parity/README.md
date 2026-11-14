# Parity capture harness

Runs the same scripted scenario against the reference Tauri app and a candidate (GPUI) app in a
Docker desktop (XFCE + Xvnc + noVNC), saves named screenshots and 30 fps frame sequences, and
compares two result trees. Everything on the host is a `docker exec` into the container.

## Container requirements

xdotool, imagemagick (`import`, `montage`, `compare`), ffmpeg (x11grab), bun (reference bridge only).
The reference container `inventory-app` (image `inventory-dev:1`) has them. Display `:1`, 1500x950.

One-time desktop setup (single workspace, no XFCE grab of Ctrl+Alt+Arrow; without this a scripted
chord can switch workspaces and the window vanishes):

    PARITY_CONTAINER=inventory-app tools/scripts/parity/prepare-desktop.sh

## Files

| file | purpose |
| --- | --- |
| `lib.sh` | helpers sourced by scenarios: `attach move click rclick dblclick drag scroll key type_ settle shot shot_root record js` |
| `run-scenario.sh` | `run-scenario.sh <reference|candidate> <scenario> <out-dir>` |
| `scenarios/*.sh` | scenario steps, window-relative pixel coordinates at 1200x800 |
| `snapshot.sh` | `save|restore [name]` of daemon home + webview storage of the reference app, then relaunch |
| `reset-reference.sh` | reference only: reset sidebar width/open and active project over the bridge, reload |
| `dump-styles.sh` | reference only: `getComputedStyle` and geometry of selected elements to JSON |
| `compare.sh` | per-name diff of two result trees (RMSE, diff images, report.tsv) |
| `contact-sheet.sh` | montage of a result directory for eyeballing |
| `bridge-eval.mjs` | the Tauri MCP bridge client used by `js` (port 9223) |

## Run the reference

    export PARITY_CONTAINER=inventory-app
    tools/scripts/parity/snapshot.sh restore golden      # identical data and layout every run
    tools/scripts/parity/run-scenario.sh reference 10-sidebar out/
    tools/scripts/parity/run-scenario.sh reference 20-settings out/

`snapshot.sh restore` relaunches `/root/launch-ref.sh` (the dev build with `CROWBAR_HOME=/root/data/home`;
the Vite dev server must be up on :5399 in the container, see the spec's "Reference environment").
Scenarios mutate state (new tabs, chats): restore the snapshot before each scenario.

## Run a candidate

Start the candidate app in a container with the same desktop, then:

    export PARITY_CONTAINER=<candidate container> PARITY_WINDOW='<window title substring>'
    tools/scripts/parity/prepare-desktop.sh
    tools/scripts/parity/run-scenario.sh candidate 10-sidebar out/

The candidate must open a window whose title contains `PARITY_WINDOW`, show the same seeded data
(restore the same daemon home snapshot into it), and handle the same input. `attach` resizes the
client area to 1200x800 and moves it to (5,40); captures use `import -window <id>` (client area only,
no decorations), so the OS title bar never enters a diff.

## Output

`out/<target>/<scenario>/<name>.png` (1x client area), `out/<target>/<scenario>/frames/<name>/f-NNNN.png`
(30 fps, frame 1 is the recording start), and `index.tsv` (name, file, kind, note) in each scenario directory.
`kind` is `shot` (client area), `root` (whole desktop; needed for native OS menus and select popups, crop the
app region before diffing) or `frames`.

## Compare

    tools/scripts/parity/compare.sh out/reference/10-sidebar out/candidate/10-sidebar diff/10-sidebar

Writes `report.tsv` (name, RMSE normalised 0..1, pixel count differing), a diff PNG per name, and
exits non-zero if any name exceeds `THRESHOLD` (default 0.02) or is missing. Frames are compared by
index; compare animations by duration and easing against the spec's animation tables, not pixel-exact.

## Known behaviours of the reference

- A freshly launched WebKit window paints black until its size changes once; `attach` does that nudge.
- A zero-length mouse press is ignored by the webview; `click` holds the button for 80 ms after a 250 ms hover.
- Dragging from the empty top-bar region moves the OS window (Tauri drag region); start drags on a control.
- Sidebar chrome is opaque on Linux (no compositor, `--chrome-bg` opaque); macOS translucency is not reproducible here.
- Native menus (context menus, select popups) are separate X windows: use `shot_root`.
- `js` needs the debug bridge and synchronous scripts that start with `return`.
