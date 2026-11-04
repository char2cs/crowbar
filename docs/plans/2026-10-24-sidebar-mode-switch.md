# Sidebar mode switch and header restyle

Approved mockup: the "Chosen direction" board of https://claude.ai/artifact/8mDA9CS5H82w3fpq3xYc2U.
Out of scope: the pane header redesign (title, `repo/parent/branch` subtitle, URL chip) — its own plan.

## Target

The sidebar column runs the full window height. Top to bottom:

1. **Header line** (same line as the view-tab strip): the sidebar toggle on the inner edge (the side facing the panes) and the Settings button. Nothing else.
2. **Project switcher** (first line of the body): the active project's icon and name (class `font-heading`), then the other projects' icons (the active one is not repeated), then `+`. Each project keeps its own icon (`ProjectIconMark`).
3. **Mode switcher**: the old segmented control (`sidebar-tab-bar.tsx`, commit 6d7e51f29): Chats / Files / Git; the active segment shows a filled icon and its label, the rest icon only. A dot on the Chats segment when another chat needs attention while Files or Git is showing.
4. **Panel**: exactly one of Chats (`SidebarTreeSurface`), Files, Git, full height. All three stay mounted; inactive ones are `hidden`, so scroll position and folded rows survive a switch.
5. **Foot**: the build/version/fps readout. While the sidebar is hidden, the readout and the reopen toggle live at the end of the view-tab strip (`view-tabs-row.tsx` already does this for a hidden sidebar).

The floating Files/Git card and everything that serves it are deleted. The `sidebarMode` (`docked | auto-hide`) setting is removed with it.

## Auto-switch

Focus moves the mode: editor tab focused -> Files; diff or commit tab -> Git; chat column or terminal -> Chats. These are surfaces inside a pane (the IDE sector's tab bar), not the project's view tabs. It acts on transitions between surface kinds only, so a click on a segment holds until focus next crosses into a different kind of surface. The store owns the mode; the listener is event-driven, with no resync effect. Today `use-sidebar-reveal.ts` already listens for focus in `[data-editor-view]` to reveal the card; that listener becomes the mode switch. `use-sidebar-tab-keyboard.ts` shortcuts stay and write to the same store.

## Rules the mode must obey

- Git is available only for a workspace with a repo (`hasRepoWorkspace`). When the active mode becomes unavailable, fall back to Chats.
- With no focused workspace (empty stage) the bar still renders; Files and Git are disabled; Chats stays usable.
- Default mode `'chats'`. The last mode persists across restarts alongside the collapsed-rows state (`lib/persistence/sidebar-ui.ts`).

## Delete in the same change

- `components/layout/sidebar-carousel.tsx`, `sidebar-carousel-head.tsx`, `use-card-resize-drag.ts`, `sidebar-card-height.ts`, `use-carousel-scroll-sync.ts`
- `buildSidebarCardStyle` and `SIDEBAR_CARD_GAP` in `features/panes/utils/pane-border.ts` if nothing else uses them
- the `--card-bottom-inset` plumbing in `ide-shell.tsx` and `components/sidebar/space-scroller.tsx`
- the `sidebarMode` setting: type, default, Appearance control, and everything that read it
- `components/layout/context-pill-model.ts` and its test
- `SidebarSpaceMarks` once the new switcher replaces it
- `SidebarCarouselFilesPanel` is renamed to a files panel, not rewritten

## Lanes

Each lane is its own worktree and branch, starts with a failing test, extends the unit's existing test file under `web/src/__tests__/` mirroring `src/`, deletes what it makes unused, and ends with a commit. Targeted vitest only, never the full suite.

| Lane | Work | Starts |
|---|---|---|
| D | Delete the context pill model and its test | now |
| A | Mode core: store default and persistence, mode bar, panel availability and fallback, body switch, delete the card, the `sidebarMode` setting and the inset plumbing | now |
| B | Header and project switcher: header reduced to toggle and Settings, new project switcher line, readout moved to the foot, readout rule in `view-tabs-row.tsx` | after A merges |
| C | Auto-switch: focus listener, surface markers, tests | after A merges |

B and C touch disjoint files and run in parallel once A is on the branch. Lanes merge by `git cherry-pick`, in a window between lanes.

## Verification

Targeted vitest files, `bun tsc --noEmit` (never `bunx tsc`), eslint with `--max-warnings 0` on touched files, `bun run knip`, prettier on touched files. Then live in `make dev-desktop` through the Tauri driver: two chats in split panes, every mode, focus moves in each surface kind, an unavailable mode, the empty stage, a hidden sidebar (readout and toggle in the tab strip), a sidebar on the left, and space switching. Unit tests alone do not cover this.
