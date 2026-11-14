# Shell surface inventory (web/src): reachable vs dead

Revision 2: DOM-level pass via the Tauri MCP bridge in Docker (see Method).

Scope: React shell under `web/src`. Purpose: keep/cut list for the Tauri+React to GPUI migration.

## Method and confidence (read first)

Two passes. Pass 1 (release app in `crowbar-novnc`, xdotool + screenshots, no DOM) is superseded by pass 2.

- **Pass 2, DOM-level.** Own image `inventory-dev:1` (docker/dev-linux Dockerfile minus gh/claude, plus nodejs, xdotool,
  imagemagick), container `inventory-app` (noVNC on 127.0.0.1:6091), volumes `inventory-src` (copy of the worktree's
  tracked files, so nothing was written into the repo), `inventory-cargo`, `inventory-home`. `crowbar-novnc` untouched.
  Debug Tauri build (`tauri dev --no-watch`, Vite on :5399 via `bun --bun x vite`, because the image's Node 18 lacks `util.styleText`)
  with `CROWBAR_HOME=/root/data/home`; the bridge listened on :9223 and I drove it with `execute_js` (DOM reads/clicks,
  same protocol as `scripts/e2e/lib/driver.mjs`) plus xdotool for real mouse/keyboard and screenshots.
- **Bridge quirk:** `execute_js` times out while a Base UI menu/popup is open (it recovers after Escape/click-away). Menus were
  therefore verified by screenshot, not DOM.
- **No provider CLIs in the image**, so no real agent turn ran: chat transcript, composer-with-provider, Agents tab with
  installed agents, permission prompts are static-only. Provider homes untouched. No production dirs.
- **knip:** `bun --bun x knip` (the configured run) reports **0 issues**; with `ignoreIssues` removed it reports
  **50 unused exports, 29 unused types, 6 duplicate exports** (the baseline hides them; list in "Static cross-check").
  My own import graph found 4 unreachable files, matching knip's file result.
- Labels below: **LIVE** = observed in the running debug app (DOM or screenshot); **STATIC** = import-graph/knip/grep only.
- Environment artefacts to ignore: Ctrl+Alt+Arrow (pane navigate) is swallowed by XFCE as a desktop switch; the dev build adds
  a Developer settings tab and the sidebar build readout, which a release build lacks (the release app showed no readout).

## Headline finding

At file level the code is clean: only 4 non-test files are unreachable from any entry
(`lib/perf/budget.ts`, `lib/workspace/branch-workspace.ts` (both test-only, knip `files` baseline),
`types/window.d.ts`, `vite-env.d.ts` (ambient, fine)). The dead weight is **inside reachable files**:
test-only exports, stubs wired into UI, and baseline-hidden exports. There is no command palette in the app.

## Reachability matrix (pass 2)

| Surface | Status | Evidence |
|---|---|---|
| OOBE: tagline, System check (git ok, gh/glab optional), Add first space (typed-path dialog) | LIVE | walked end to end; `#/oobe` redirects to home once a space exists |
| `/add-host` route | LIVE (partial) | renders brand frame only when a local daemon exists (guard); real entry is the Add-host dialog |
| Add-host dialog (Create-space daemon picker, Settings > Hosts "Add host") | STATIC + button seen LIVE | picker popup did not open under xdotool; dialog body not driven |
| Sidebar Chats / Files / Changes modes; ctrl+1/2/3 | LIVE | Files tree ("demo"), Changes panel (merge into main, commit form, changed files) |
| Space "+" create panel; "More..." menu (Import a repo / Create a new folder / Delete space) | LIVE | |
| Add repository dialog (typed path), Import branches dialog | LIVE | imported local repo, listed remote branches |
| Repo row menu (Rename, Import branches, New folder, New thread in Terminal, Delete Repo); workspace row menu (New folder, New thread in Terminal) | LIVE | right-click screenshots |
| Inline new-branch row (repo header branch icon) -> branch workspace + chat | LIVE | |
| Chat view: empty-state composer (GOAL/WHERE/KEEP), Full Auto permission picker, Chat/Terminal toggle, editor-area toggle | LIVE | no provider, so no transcript |
| Chat transcript, streaming, provider/model picker, attachments | STATIC | needs a provider CLI |
| New-tab page (Terminal, Browser, File, Review Branch only on branch workspace) | LIVE | |
| Terminal pane (PTY, typed `echo`, got output) | LIVE | |
| Browser pane (back/forward/reload/pick element/address) | LIVE (chrome only) | page load and element pick not exercised |
| Editor pane (Monaco textbox, language picker, LSP status, find) | LIVE (DOM) | editor body painted blank in screenshot; DOM present |
| Review Branch pane: Conversation / Files changed, comment box, side-by-side vs unified, Viewed | LIVE | untracked `new.txt` showed "Could not load this diff / Retry" (defect candidate) |
| Tab bar: new tab, tab context menu (Pin, Rename, Copy Path, Reveal in Finder, Close*), drag-reorder with ghost | LIVE | "Reveal in Finder" on Linux is a macOS-ism; native WebKit menu (Back/Reload/Inspect Element) leaked once over the sidebar |
| Console dock (Ctrl+`) with daemon log, dock position, mode, clear | LIVE | |
| Settings dialog: Appearance, Editor, Files, Git, Terminal, Agents, Active sessions, Keybindings, Hosts, Developer(dev only) | LIVE (DOM text) | Browser tab click raced; content unconfirmed. Keybindings page states IDE presets "not yet provided" |
| Keyboard: Ctrl+T new tab, Ctrl+J terminal, Ctrl+Shift+N new file, Ctrl+N, Ctrl+/ chat/terminal toggle, Ctrl+W close, Ctrl+1/2/3, Ctrl+` | LIVE | tab list diffed before/after |
| Keyboard with no observable effect here: Ctrl+Alt+N, Ctrl+Shift+T (reopen), Ctrl+O, Ctrl+Shift+E (pick), Ctrl+=/-/0 (zoom), Ctrl+S/Shift+S, pane-navigate | bound (STATIC) / inconclusive | needs file, browser focus, visual check, or non-XFCE desktop |
| Command palette | does not exist | grep + UI |
| Commit-diff pane (`CommitDiffPane`) | **STATIC: no UI opens it** | no non-test code creates a `commitDiff` buffer; only render/persist/find handlers reference it |
| Markdown / HTML preview panes | LIVE-reachable by code (`breadcrumb.tsx` toolbar for .md/.html files) | not driven (needs a saved .md file) |
| Mobile webviews (`#/mobile-sidebar|settings|files`) | STATIC | native phone shell only |
| Context menu on files tree / drag-drop of sidebar rows | not driven | STATIC |

## Classification

KEEP = reachable and needed in GPUI. CUT = unreachable or stub. REDESIGN = reachable but the GPUI shell should not copy it 1:1.

### CUT (dead or stubbed, safe to drop)

| Item | Evidence | Reason |
|---|---|---|
| `CommitDiffPane`, `features/panes/components/commit-diff-pane.tsx`, the `commitDiff` pane-content type, its spec in `new-buffer.ts`, `find-open-buffer.ts`, `tab-buffer-icon.tsx`, `use-native-tabs.ts` case | Live: no UI path; static: nothing but tests create a `commitDiff` buffer | Whole surface unreachable; the largest confirmed dead feature (CUT, subject to owner confirming no planned commit-history UI) |
| Developer settings tab (`developer-settings.tsx`), sidebar `BuildReadout` (`build-readout*.ts*`, `use-readout-lines.ts`) | Present only in the debug build (`import.meta.env.DEV`/dev label); absent in the release app | Dev tooling, not product; drop from the GPUI shell |
| `extensions/ui/hooks/use-extension-actions.ts` + `extensions/ui/components/extension-toolbar-action.tsx` | header "// Stub"; hook returns `{left:[],right:[]}`, component returns `null`; rendered in `features/editor/components/toolbar/breadcrumb.tsx` | Shipped stub wired into UI; violates CLAUDE.md "no stubs that ship" |
| `extensions/registry/extension-registry.ts` (+ `extensions/themes/types.ts` "// Stub") | "Stub: extensions feature is out of scope"; returns `[]`; consumed by `language-id.ts`, `monaco-lsp-providers.ts`, `editor-status-actions.tsx` | Same; consumers contribute nothing |
| `features/git/api/git-status-api.ts` `stageHunk`, `unstageHunk` | zero uses outside own file, not even tests | No UI for hunk staging |
| `features/panes/utils/pane-command-actions.ts`: `toggleActiveEditorGroupLock`, `splitActiveEditorGroup`, `closeActiveEditorGroup`, `closeOtherEditorGroups`, `resetEditorGroupSizes`, `moveActiveEditorToAdjacentGroup` (and `splitEditorGroup`, `getShareableSplitBufferId` by extension) | only `pane-command-actions.test.ts` imports them; the keymap registry has no matching command; only `openBranchReviewForWorkspace`, `ensurePaneChatThenOpen`, `openNewTabPage`, `togglePaneEditor` are used | VS Code editor-group commands with no entry point |
| `features/panes/utils/pane-drop-actions.ts` `moveBufferToPaneDropTarget`; `pane-routing.ts` `resolveWritablePaneForBuffer`; `pane-layout.ts` `appendLeaf`, `resizeFlattenedLayout`, `findParentSplit` | test-only / self-only | Residue of old editor-group layout |
| `lib/perf/budget.ts` (`checkBudgets`), `lib/workspace/branch-workspace.ts` (`findWorkspaceForBranch`) | unreachable files; knip `files` baseline | Test-only files |
| `lib/stubs/remark-mdx.ts` `remarkMdx` | no non-test use; it is a vite alias target (`vite.config.ts`) | Keep only as build alias; export itself unused |
| Test-only exports: `getHomeWorkspacePath`, `placeProject`, `fuzzyMatch`, `IS_LINUX`, `shiftUiFontSize`, `saveToLocalStorage`, `resetDB`, `resetConnectionState`, `grammarStateOf`, `isWorkspaceEdit`, `offsetFromPosition`, `getStickyAncestorRow`, `normalizeEnvTargetFileName`, `fieldRightPadding`, `dragSubjectsFor`, `INTO_MODES`, `LinkKitStatic`, `getGitStatus`, `unstagePaths`, `assertViewIntegrity` | scan: non-test uses = 0, test uses > 0 | Exports kept for tests only; make private or delete with the test |
| Zero-use exports: `projectIdForRepo`, `PopoverTitle`, `TooltipCompound`, `getActiveWorkspaceStore` | scan: no use anywhere | Dead |
| `*ForTests` / `__reset*` exports in non-test files (`resetWindowPaneStoreForTests`, `setActiveWorkspaceStoreForTests`, `resetChatPresentationMemoryForTests`, `__resetSelectionDragForTests`, `__resetShikiTokensForTests`, `_resetTerminalFocusRegistryForTests`, `__resetPerfForTests`, `__resetWorkspaceScopesForTest`) | CLAUDE.md bans these in non-test files | Hygiene; move to test helpers |
| Settings "Developer" tab (`developer-settings.tsx`, `BuildBadgeSection`) | `import.meta.env.DEV` only; absent in release (confirmed live) | Unreachable for users |
| knip `ignoreIssues` baseline in `web/knip.json` (~130 entries, mostly "exports") | baseline hides unused exports | Each entry is a candidate; baseline may only shrink |

### REDESIGN candidates (reachable, but do not port 1:1)

| Item | Reason |
|---|---|
| Plate-based composer + transcript (`features/agent/composer/plate/*`, `transcript/plate/*`, `components/editor/*`) | Largest web-only surface; rich-text editor stack tied to React/Plate; GPUI needs native text input |
| Monaco editor stack (`features/editor/monaco/*`, `lsp/*`, shiki tokens) | Web-only; reached via New File / editor pane; decide whether GPUI ships an editor or defers to external |
| Branch review pane + diff (`features/git/components/*`, `diff/*`) | LIVE (Review Branch); heavy `@pierre`-style diff UI; redesign natively |
| Pane/split/drag tree (`features/panes/*`, `tab-bar*`, `tree-dnd`, `sidebar-split-pane`) | Reachable; large DnD machinery; GPUI layout model differs |
| Browser pane (`features/browser/*`) | LIVE chrome via New Browser; embeds a webview, which GPUI cannot host directly |
| Console dock (`features/console/*`, `TOGGLE_CONSOLE`) | LIVE via Ctrl+`; useful daemon log, keep a native equivalent |
| Mobile webviews (`mobile-sidebar-main`, `mobile-settings-main`, `mobile-files-main`, `components/layout/mobile`) | Entered only via `#/mobile-*` hash from native phone shell; not reached; out of GPUI desktop scope unless iOS is retained |

### KEEP (reachable, confirmed or highly likely)

| Item | Evidence |
|---|---|
| Sidebar chats/files/changes modes, project/thread tree, create-space panel, "New thread", "More..." | live |
| Chat view, composer, branch header, tab strip, Ctrl+N new chat | live |
| Settings dialog tabs: Appearance, Editor, Files, Git, Terminal, Browser, Agents, Active sessions, Keybindings, Hosts | live; Hosts shown only under Tauri (`isTauri()`) |
| New-tab page (Terminal, Browser, File, Review Branch) | live |
| Terminal pane (`features/terminal/*`) | LIVE (PTY verified) |
| Routes `/oobe`, `/add-host`, `_shell` landing | wired via `shell-landing.tsx`; LIVE (OOBE walked in pass 2); `/add-host` guarded by local daemon |
| Keymap commands (24 in `features/keymaps/registry.ts`: pane nav x4, tab new/close/reopen/open file, new chat, view-mode toggle, cycle provider, zoom x3, save/save-all, browser pick, console, sidebar tabs x3) | all bound; 9 verified live, rest inconclusive (see matrix) |
| Daemon crash/halt gates, unsaved-changes dialog, removal tray/confirm, detach modal, toasts, operations watcher | STATIC (event-driven); removal tray/detach not triggered |

## Static cross-check: knip with the baseline removed

`bun --bun x knip -c <copy of knip.json without ignoreIssues>`: 50 unused exports, 29 unused types, 6 duplicate exports.
Beyond those already listed above, notable unused exports: `performRenameWorkspaceBranch/Folder/HomeFolder/HomeChat/Repo`
(`components/sidebar/lib/row-actions.ts`), `edgeScrollStep`, `KIND_RANK`, `anchoredToastManager` (duplicated in
`toast.tsx` and `toast-manager.ts`), `cacheThemeForBootstrap`, `ThemeRegistry`, `ExtensionRegistry`, `defaultSettings`,
`resolveInitialSettings`, `cacheFontSettings`, `setWorkspaceScope/getWorkspaceScope`, `SidebarHeaderSearch`,
`SidebarHeaderIconButton`, unused `ui/dropdown-menu` sub-menu parts, 6 duplicate exports (e.g. `getFileName`/`getFilenameFromPath`).
All are CUT-on-sight (delete the export, or the code if only it used it); none is a user-visible surface.

## Not verifiable / follow-ups

1. Everything needing a real agent (transcript, composer with provider, permissions, Active sessions with rows) is STATIC; run a provider CLI in the container to cover it (allowed).
2. Not driven: Add-host dialog body, file-tree context menu and file DnD, sidebar row DnD, Markdown/HTML preview, Browser settings tab content, zoom/save/pick shortcuts.
3. Component-level conditional branches (platform/flag gated, `IS_MOBILE`) were not enumerated.
4. Leftover for reuse: container `inventory-app` (running), image `inventory-dev:1`, volumes `inventory-src|cargo|home`; remove when done.
