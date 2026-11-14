# Repo layout: clients behind `app/`

Status: DRAFT for owner review. Nothing is moved yet. Paths inside `panes/` are placed by what each folder does today and are confirmed during the move.

## Tree

```
crowbar/
├─ api/            Go daemon (unchanged)
├─ app/            every client the user runs
│  ├─ panes/       shared React: pane entries, libraries, components (was web/)
│  ├─ crates/      Rust shared by desktop and mobile
│  ├─ desktop/     GPUI app, Linux and macOS (new)
│  └─ mobile/      iOS app: Swift shell + Tauri (was desktop/)
├─ website/        marketing site
├─ docs/           plans, triage, protocol notes
├─ tools/          docker/, scripts/ (repo-wide tooling)
├─ legal/          LICENSING.md, NOTICE.md, licenses/
├─ README.md  LICENSE  CLAUDE.md  Makefile  .github/
```

`LICENSE` stays at the root so hosting sites detect it. The root `Makefile` only delegates to `api/`, `app/*` and `tools/`. `ARROW.md` stays until its owner says where it belongs.

## app/panes (shared React package, `@crowbar/panes`)

```
panes/
├─ package.json  vite.config.ts  tsconfig*.json  eslint.config.js  knip.json  doctor.config.ts
├─ public/
└─ src/
   ├─ entries/      one small entry per pane (chat, editor, files, review, console, terminal);
   │                each builds to its own bundle that the shell loads by URL
   ├─ features/     pane logic: agent, editor, files, file-explorer, file-system, git, terminal, console
   ├─ components/   shared UI: ui/, editor/, tree-dnd/, error-boundary
   ├─ lib/          api, ws, transport, persistence, errors, events, host-resolve
   ├─ core/ hooks/ styles/ types/ utils/
   ├─ sdk/          sdk@1 client that panes and extension panes import (shell message channel)
   └─ __tests__/    mirrors src/ (CLAUDE.md rule)
```

- Shared code never imports from `mobile/` or `desktop/`. An import-boundary lint check enforces it.
- Desktop-shell features (`layout`, `tabs`, `window`, `keymaps`, `panes` store, sidebar components, settings pages) are deleted as the native GPUI shell replaces each one. Mobile still needs a few of them, which move to `mobile/content/` during the extraction step.

## Rust workspace: one, rooted at `app/`

`app/Cargo.toml` is the only workspace for desktop and shared code (one lockfile, one set of versions), like Zed's single root workspace. Members: `crates/*`, `desktop/crates/*`, `desktop/app`. The Tauri crate in `mobile/src-tauri` stays outside it (its own lockfile, `exclude`d) and depends on the shared crates by path.

Rule: shared crates in `app/crates/` never depend on gpui or tauri, so both apps can use them. Crate names are snake_case; logic and view are separate crates (`terminal` and `terminal_view`) so logic tests run headless.

## app/crates (shared Rust)

```
crates/
├─ transport/   crowbar:// protocol, host connections, tokens, TLS pin   (from api_proxy, hosts, host_tokens, tls_pin)
├─ protocol/    control frames, pane and extension messages, settings schema types
└─ model/       layout and view-state data types both shells implement
```

## app/desktop (GPUI)

```
desktop/
├─ app/         binary: window, lifecycle, menus (thin, like Zed's `zed` crate)
├─ crates/
│  ├─ shell/          sidebar, tab bar, layout, command palette
│  ├─ pane_host/      pane registry, lifecycle, webview pool
│  ├─ webview/        the one platform crate: wry, GTK thread, window-handle wrapper
│  ├─ daemon_supervisor/  supervision, health, recovery (from sidecar/*)
│  ├─ settings_ui/    native settings UI built from the daemon schema
│  ├─ terminal/       alacritty_terminal wrapper and daemon frame feed (no gpui)
│  ├─ terminal_view/  GPUI view for the terminal
│  └─ ui/             shared GPUI components and theme
├─ vendor/gpui/  patched gpui + xattr (patch also sent upstream); a [patch.crates-io] at the workspace root
├─ assets/       icons, fonts
├─ packaging/    macOS dmg + signing, Linux packaging (from desktop/scripts)
├─ tests/
└─ Makefile
```

An editor crate is added later (`gpui-component` editor is the candidate).

## app/mobile (iOS)

```
mobile/
├─ ios/          Swift package: chrome, pickers, settings (was desktop/ios-chrome/ios)
├─ ios-plugin/   Rust Tauri plugin that bridges Swift (was desktop/ios-chrome src, build.rs)
├─ content/      React phone layout and the feeds to Swift; imports @crowbar/panes
├─ src-tauri/    Cargo.toml, tauri confs, capabilities, icons, entitlements
├─ icons-compiler/  icons-src/  scripts/ (ios install, release)
└─ Makefile
```

`content/` is created in the extraction step. Until then the phone layout stays in `panes/src/mobile/`.

## tools, docs, legal

- `tools/docker/`: Dockerfile, compose, dev-linux. `tools/scripts/`: e2e, meter, perf, test-name checks, service install.
- `docs/` stays as is (plans, triage, release and signing notes).
- `legal/`: `LICENSING.md`, `NOTICE.md`, `licenses/`.

## Move order

1. Commit the dead-code removal.
2. One move-only commit: `web/` to `app/panes/`, `desktop/` to `app/mobile/`, `docker/` and `scripts/` to `tools/`, license files to `legal/`, with every path fixed (CI, Makefiles, configs, CLAUDE.md, docs).
3. Create `app/crates/` and `app/desktop/` and port the Rust that moves out of the old Tauri crate.
4. Extract `mobile/content/` and add the import-boundary check.
