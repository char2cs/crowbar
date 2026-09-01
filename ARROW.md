# Crowbar

_The IDE where agents do the heavy lifting._

Coding agents can now produce more code in an afternoon than anyone can carefully read, and the tooling around them has not kept up. They run loose in the working tree you are also using, or behind a web UI far from the repository they are editing; you are married to whichever vendor's CLI you happened to open; and the review that should decide whether any of it is good happens somewhere else entirely, in a browser tab, against a diff with no trace back to the conversation that produced it.

Crowbar is a local-first desktop IDE built around that gap. Import a repository and every branch becomes its own managed git worktree; start a chat and an agent goes to work inside one, in a real terminal you can watch and type into. Move that same conversation between Claude Code and Codex whenever one suits the task better. When it stops, the branch's review is already there — diff, inline comment threads, PR status — and nothing it did ever touched your own checkout. It runs entirely on your machine: one Go daemon, a React frontend, and a Tauri shell around them.

#### What's in this repo

Everything. `api/` is the Go daemon that owns worktrees, agent sessions, terminals, LSP and search; `web/` is the React frontend; `desktop/` is the Tauri shell that hosts it; `docker/` is the same daemon serving a prebuilt bundle for self-hosting. The daemon is the product — the desktop app is a window onto it, which is why the Docker image serves the identical thing with no desktop app involved.

#### What it does

- **Diff review** — every branch carries its own diff, inline comment threads and PR status.
- **Provider switching mid-conversation** — move a live chat between Claude Code and Codex freely.
- **Deterministic workflows** _(WIP)_ — repeatable, declaratively-defined agent runs.
- **And more yet to come.**

---

## Switch providers mid-conversation

Claude Code and Codex both run as first-class providers, and a chat is not bound to whichever one opened it. Switch mid-conversation and the outgoing CLI is asked to exit cleanly — so it flushes its own transcript instead of losing its last turn — while the incoming one picks the thread up in the same workspace. Providers can be enabled, disabled and prioritised globally, and each one reports whether it is actually installed on this machine.

That is possible because a provider is a YAML descriptor, not an integration. Everything Crowbar needs to know about a CLI — how to spawn it, how to resume a session, which hooks report progress — is declared in one file:

```yaml
id: claude
display_name: Claude

spawn:
  cmd: claude
  interactive_required: true
  # A chat opens in auto mode: the pane is a working agent, and being
  # prompted for each edit makes it useless.
  args: ["--permission-mode", "auto"]

session:
  resume: { arg: "--resume {id}" }

hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt: { message: prompt }
```

Two descriptors ship today — `claude.yaml` and `codex.yaml`. In principle a third CLI is a third file; in practice only those two have been proven end to end, and each carries the sharp edges its vendor actually has (Codex, for instance, runs under a kernel sandbox that has to be handed back network access to reach its own hooks).

## Diff review

Every branch carries its own review: the diff, inline comment threads anchored to lines, and PR status — in the same window as the agent that wrote the code. Reading the work and going back to the agent about it is one motion instead of two tools and a browser tab.

## Deterministic workflows _(WIP)_

Not every task deserves a conversation. Workflows are declaratively-defined agent runs — same steps, same order, every time — for the work you would otherwise re-explain in prose on each attempt. This one is in progress, not shipped.

---

## Every branch is a worktree

Importing a repository provisions a managed worktree per branch, laid out by remote and branch name so the path tells you what it holds:

```
~/.crowbar/projects/<project>/github.com/acme/api/
├── main/worktree              # protected → locked, provisioned at import
├── develop/worktree           # protected → locked
└── feature/rate-limit/worktree
```

Protected branches are permanently checked out in their own locked worktree, so `develop` is always somewhere on disk in a known state. Everything else forks from **origin's** tip of its parent, not from whatever your local ref happens to be — a branch created at 9am does not silently start from yesterday's `develop`.

Agents only ever run inside a worktree. Two of them cannot tread on each other, and neither can reach the checkout you are working in.

## The terminal is a screen model, not a stream

The daemon runs the VT emulation itself and keeps an authoritative screen model per session; the frontend renders row diffs of that model. Terminals and agent chats therefore survive pane switches, workspace switches, and reloads of the whole webview — the session state was never in the browser to begin with. It also means an agent's PTY is a first-class object the daemon can reason about, rather than bytes flying past.

## The rest of the IDE

Monaco editor with LSP, project-wide search, file explorer, integrated terminals, git history and branch operations — import a branch that only exists on origin, rename a workspace and have git, disk and record move in lockstep. Markdown opens in a rich editor with a Source view a click away. Switching Crowbar's theme is pushed down to the sessions themselves, so a CLI running in a pane follows the app instead of staying dark against a light window.

## More to come

Stated plainly, because a description that blurs these is worthless:

- **Deterministic workflows** — in progress, as above.
- **Per-project review memory** — the thesis this is all pointed at: an agent reads the comments you leave on real reviews, extracts the principles behind them, and feeds those into future runs, so the reviewer starts catching what you catch and you review less over time.

Everything else described above ships today.

## Extending Crowbar, through Quiver

Crowbar already ships through Quiver — this file is how. The next step is for the pieces Crowbar is _made of_ to travel the same way — published by anyone, installed by anyone, with no marketplace to be admitted to and no plugin API to be blessed into:

- **Provider descriptors.** Publish the YAML for any CLI and it runs beside the ones that ship, with the same chats, workspaces and mid-conversation switching. Provider-agnostic in the literal sense, rather than "we support the two we integrated".
- **Deterministic workflows.** Share a workflow the way you share a repository, and run someone else's without rebuilding it from a blog post.
- **LSP servers.** Language support as something you install, not something we bundled.
- **Themes.** Same story.

This is the direction, not the state.

---

## Arrow manifest

The block below is what Quiver actually reads. It resolves an install to a universal macOS DMG or an amd64 AppImage on Linux, both pulled straight from the matching GitHub Release — there is no daemon for Quiver to supervise, since Crowbar is a desktop app the user launches and the Tauri shell spawns its own `crowbar-api` sidecar once installed.

```arrow
schema: "arrow@v0"

# ─── Package Arrow: Crowbar ───
#   Lifecycle: absent → ready → removed
#              No execute/stop: Crowbar is a desktop application the user launches, not a
#              daemon Quiver supervises. The Tauri shell spawns its own crowbar-api sidecar.
#   Platform:  darwin/amd64, darwin/arm64 — universal DMG; Crowbar.app copied to /Applications
#              linux/amd64               — AppImage, launcher in ~/.local/bin, desktop entry
#   Excluded:  linux/arm64, windows/*    — the release pipeline builds no artifact for them
#                                          (.github/workflows/stable-release.yml ships only a
#                                          universal macOS DMG and an amd64 AppImage/deb)
#
#   This file carries no version of its own. ${REF} is the git ref Quiver resolved, and the
#   release workflows rename every bundle to match it, so one URL serves every channel:
#   a bare `crowbar` resolves to the latest stable tag, `crowbar@nightly` to the rolling
#   develop build. Nothing here changes when a release is cut.

metadata:
  name: crowbar
  description: >-
    Local-first desktop IDE where coding agents do the heavy lifting. Every branch becomes its
    own managed git worktree, Claude Code and Codex are interchangeable mid-conversation, and
    the diff review — inline comment threads and PR status — lives in the same window as the
    agent that wrote the code.
  license: "AGPL-3.0-only OR LicenseRef-Commercial"
  url: "https://github.com/char2cs/crowbar"
  maintainers:
    - name: char2cs
      url: "https://char2cs.net"
  credits:
    - name: Rabbyte Software
      url: "https://github.com/rabbytesoftware"
  media:
    icon: "https://raw.githubusercontent.com/char2cs/crowbar/develop/.github/crowbar-icon.png"
    banner: "https://raw.githubusercontent.com/char2cs/crowbar/develop/.github/crowbar-banner.png"
  tags:
    - ide
    - development
    - agents
    - git
    - desktop
    - code-review

targets:
  "darwin/*":
    requirements:
      cpu_cores: 2
      ram_gb: 4
      disk_gb: 2

    lifecycle:
      install:
        - type: fetch
          title: "Download the Crowbar universal disk image"
          url: "https://github.com/char2cs/crowbar/releases/download/${REF}/Crowbar_${REF}_universal.dmg"
          to: "${INSTALL_PATH}/Crowbar.dmg"
          timeout: 15m

        # Mount, copy and unmount in one command so the disk image is always detached, even
        # when the copy fails — a step that aborts the run would never reach a separate
        # detach step and would leave /Volumes/crowbar-quiver mounted.
        - type: run
          title: "Install Crowbar.app into /Applications"
          command: "hdiutil attach -quiet -nobrowse -mountpoint /Volumes/crowbar-quiver \"${INSTALL_PATH}/Crowbar.dmg\" && rm -rf /Applications/Crowbar.app && cp -R /Volumes/crowbar-quiver/Crowbar.app /Applications/Crowbar.app; RC=$?; hdiutil detach /Volumes/crowbar-quiver -quiet >/dev/null 2>&1; exit $RC"
          timeout: 10m

        - type: run
          title: "Clear the quarantine flag and remove the disk image"
          command: "xattr -dr com.apple.quarantine /Applications/Crowbar.app >/dev/null 2>&1; rm -f \"${INSTALL_PATH}/Crowbar.dmg\""
          timeout: 2m
          exit_on_failure: false

      update:
        - type: fetch
          title: "Download the requested Crowbar release"
          url: "https://github.com/char2cs/crowbar/releases/download/${REF}/Crowbar_${REF}_universal.dmg"
          to: "${INSTALL_PATH}/Crowbar.dmg"
          timeout: 15m

        - type: run
          title: "Replace Crowbar.app in /Applications"
          command: "hdiutil attach -quiet -nobrowse -mountpoint /Volumes/crowbar-quiver \"${INSTALL_PATH}/Crowbar.dmg\" && rm -rf /Applications/Crowbar.app && cp -R /Volumes/crowbar-quiver/Crowbar.app /Applications/Crowbar.app; RC=$?; hdiutil detach /Volumes/crowbar-quiver -quiet >/dev/null 2>&1; exit $RC"
          timeout: 10m

        - type: run
          title: "Clear the quarantine flag and remove the disk image"
          command: "xattr -dr com.apple.quarantine /Applications/Crowbar.app >/dev/null 2>&1; rm -f \"${INSTALL_PATH}/Crowbar.dmg\""
          timeout: 2m
          exit_on_failure: false

      uninstall:
        - type: run
          title: "Remove Crowbar.app"
          command: "rm -rf /Applications/Crowbar.app \"${INSTALL_PATH}/Crowbar.dmg\""
          timeout: 2m
          exit_on_failure: false

  "linux/amd64":
    requirements:
      cpu_cores: 2
      ram_gb: 4
      disk_gb: 2

    lifecycle:
      install:
        - type: fetch
          title: "Download the Crowbar AppImage"
          url: "https://github.com/char2cs/crowbar/releases/download/${REF}/Crowbar_${REF}_amd64.AppImage"
          to: "${INSTALL_PATH}/Crowbar.AppImage"
          timeout: 15m

        - type: fetch
          title: "Download the Crowbar icon"
          url: "https://raw.githubusercontent.com/char2cs/crowbar/${REF}/desktop/src-tauri/icons/128x128@2x.png"
          to: "${INSTALL_PATH}/crowbar.png"
          timeout: 2m
          exit_on_failure: false

        # APPIMAGE_EXTRACT_AND_RUN=1 lets the AppImage run on hosts without FUSE.
        - type: run
          title: "Install the crowbar launcher into ~/.local/bin"
          command: "chmod +x \"${INSTALL_PATH}/Crowbar.AppImage\" && mkdir -p \"$HOME/.local/bin\" && printf '#!/bin/sh\\nAPPIMAGE_EXTRACT_AND_RUN=1 exec \"%s\" \"$@\"\\n' \"${INSTALL_PATH}/Crowbar.AppImage\" > \"$HOME/.local/bin/crowbar\" && chmod +x \"$HOME/.local/bin/crowbar\""
          timeout: 1m

        - type: run
          title: "Register the Crowbar desktop entry"
          command: "mkdir -p \"$HOME/.local/share/applications\" && printf '[Desktop Entry]\\nType=Application\\nName=Crowbar\\nComment=The IDE where agents do the heavy lifting\\nExec=%s\\nIcon=%s\\nTerminal=false\\nCategories=Development;IDE;\\n' \"$HOME/.local/bin/crowbar\" \"${INSTALL_PATH}/crowbar.png\" > \"$HOME/.local/share/applications/crowbar.desktop\""
          timeout: 1m
          exit_on_failure: false

      update:
        - type: fetch
          title: "Download the requested Crowbar release"
          url: "https://github.com/char2cs/crowbar/releases/download/${REF}/Crowbar_${REF}_amd64.AppImage"
          to: "${INSTALL_PATH}/Crowbar.AppImage"
          timeout: 15m

        - type: run
          title: "Make the new AppImage executable"
          command: "chmod +x \"${INSTALL_PATH}/Crowbar.AppImage\""
          timeout: 1m

      uninstall:
        - type: run
          title: "Remove Crowbar"
          command: "rm -f \"$HOME/.local/bin/crowbar\" \"$HOME/.local/share/applications/crowbar.desktop\" \"${INSTALL_PATH}/Crowbar.AppImage\" \"${INSTALL_PATH}/crowbar.png\""
          timeout: 2m
          exit_on_failure: false
```

---

## Contributing

Crowbar is open-source and contributions are welcome. Building it needs Go, [Bun](https://bun.sh) and Rust.

### Prerequisites

On a clean macOS install:

```bash
# Xcode Command Line Tools — git, make, and the clang toolchain the Tauri/Rust build needs
xcode-select --install

# Homebrew — skip if already installed
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Go 1.26.2+ (matches api/go.mod)
brew install go

# Bun, for the web frontend
curl -fsSL https://bun.sh/install | bash

# Rust, for the Tauri desktop shell (rust-version 1.85+ per desktop/src-tauri/Cargo.toml)
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

Open a new shell (or re-source your profile) so `go`, `bun` and `cargo` are all on `PATH`, then clone the repo:

```bash
git clone https://github.com/char2cs/crowbar.git
cd crowbar
```

| Command              | Description                                       |
| -------------------- | ------------------------------------------------- |
| `make dev`           | Daemon, web and desktop together, with hot reload |
| `make dev-desktop`   | Just the Tauri app                                |
| `make build`         | Web bundle, Go daemon, and desktop app            |
| `make test`          | api + web + desktop                               |
| `make lint`          | Lint everything                                   |
| `make test-coverage` | Tests with coverage reports                       |
| `make docker-up`     | Self-hosted daemon at `localhost:3737`            |

Before opening a PR, always run:

```bash
make pr-checks
```

That is lint, coverage and build — the same suite CI runs.

Every `dev` target roots Crowbar's state at `<repo>/.crowbar` instead of `~/.crowbar`, so a dev instance never collides with an installed one. Tests deliberately do not inherit that override.

---

## Naming

Crowbar follows the Valve-universe naming schema used across the rabbytesoftware ecosystem. The crowbar is Gordon Freeman's tool — simple, brutally effective, works in any environment, and the first thing you reach for. That is the right metaphor for a development tool.

---

## License

Crowbar is dual-licensed: [**AGPL-3.0**](LICENSE) or a **commercial license**.

Use it, modify it, self-host it — freely, under the AGPL. The catch is AGPL § 13: if you modify Crowbar and let others interact with it over a network, you have to publish your changes. Running it on your own machine for your own work triggers nothing. If that does not work for you, a commercial license removes the copyleft obligations — see [LICENSING.md](LICENSING.md).

Third-party components keep their own licenses; see [NOTICE.md](NOTICE.md).

---

## Stay Connected

- [Rabbyte GitHub](https://github.com/rabbytesoftware)
- [char2cs](https://char2cs.net)
