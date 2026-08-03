# P3.56 — Liveness audit of the 48 ported surfaces

Audit only. No code, mapping, or oracle files were modified; this file is the
only change. Method: for each of the 48 names in
`Surface::names()` (`native/crates/crowbar-app/src/surface.rs`), resolve its
React original (from the surface module's own doc comment and/or
`native/mapping/<name>.md`), then trace importers of that original from the
app root (`web/src/main.tsx` -> router -> routes -> ...), checking **both**
`@/components/...` and relative (`./`, `../`) import spellings at every hop.
A component is LIVE only if some import chain reaches a JSX call site that
actually renders on a path reachable from the root with no dead condition in
between (or a route/flag names a real gate, in which case CONDITIONAL).

## Control

**tooltip** — `web/src/main.tsx` imports `TooltipProvider` from
`@/components/ui/tooltip` directly and wraps `<RouterProvider>` in it (line 7,
100-107). This is as live as anything in the app: every route renders inside
it. Confirms the method reports a known-live component as LIVE. See tooltip's
row below for the full picture (Tooltip/TooltipTrigger content usage too).

## Headline

(filled in as rows complete; final tally at the bottom)

## Rows

| Surface | React original | Verdict | Evidence |
|---|---|---|---|
| alert-dialog | `web/src/components/ui/alert-dialog.tsx` (`AlertDialog`/`AlertDialogContent`/Header/Title/Description/Footer/Close) | CONDITIONAL | Sole importer: `web/src/features/git/components/review-thread-item.tsx:22-28,389-421`. Chain: review-thread-item.tsx → `use-review-annotations.tsx` → `features/git/components/diff/review-code-view.tsx` → `review-diff-tab.tsx` → `{commit-diff-pane.tsx \| branch-review-pane.tsx}` → `pane-container.tsx` → `pane-node-renderer.tsx` → `split-view-root.tsx` → `workspace-layout-root.tsx` → `workspace-view.tsx` → `workspace-host.tsx` → `ide-shell.tsx:13,184` → `routes/_shell.tsx:5` (`component: IDEShell`). Gate: `pendingDelete !== null` — user clicks "Delete" on a comment/thread row while a review diff tab is open. |
| avatar | `web/src/components/ui/avatar.tsx` (`Avatar`/`AvatarImage`/`AvatarFallback`) | CONDITIONAL | Two importers: `review-thread-item.tsx:95-97` (`MessageAvatar`, same chain as alert-dialog minus the delete click — gate: an open review diff tab with ≥1 thread message) and `components/layout/repo-icon-popover.tsx` (via `repo-section.tsx` → `workspace-tree.tsx` → `sidebar-carousel.tsx` → `ide-shell.tsx` → `routes/_shell.tsx`, gated on opening a repo-icon popover). |
| badge | `web/src/components/ui/badge.tsx` (`Badge`) | CONDITIONAL | `review-thread-item.tsx:146-150` renders `<Badge variant="outline">agent</Badge>` only when `display.isAgent`. Grepped every `isAgent` write in non-test source: the only web-app write is `use-review-annotations.tsx:208,237`, hardcoded `isAgent: false` for every reply the web UI itself creates — `isAgent: true` can only arrive on a `ThreadDTO` from the daemon (an agent posting a review comment via the API, outside `web/src`). Reachable in real production use, but no button/action inside the web app produces the true case itself — flagged UNCERTAIN-adjacent below. |
| button | `web/src/components/ui/button.tsx` (`Button`) | LIVE | `routes/_shell.tsx:5` → `IDEShell` (`ide-shell.tsx`). `hasNavScreen` defaults false, so `<SidebarProjectHeader />` renders by default; `components/layout/sidebar-project-header.tsx:7,27,46,59,72` renders four `<Button>` elements unconditionally as default IDE chrome — no data/edge-case gate, same category as the `tooltip` control. |
| card | `web/src/components/ui/card.tsx` (`Card`/`CardHeader`/`CardTitle`/`CardContent`) | CONDITIONAL | Sole importer: `components/error-boundary.tsx:4,36-49`, the fallback branch of `ErrorBoundary`, reached only via `getDerivedStateFromError` (a render-phase throw) on a boundary given no `fallback` prop. Fallback-less `<ErrorBoundary>` sites: `routes/__root.tsx:17` (wraps the whole route `<Outlet>` — root itself), `ide-shell.tsx:152,162`, `sidebar-carousel.tsx:130`. Gate: an uncaught render-phase exception in one of those subtrees; no currently-known live throw lands there (the one documented throw, in `mermaid-diagram.tsx`, is caught by a *fallback-bearing* boundary instead). Real code, unobserved in normal use. |
| checkbox | `web/src/components/ui/checkbox.tsx` (`Checkbox`) | CONDITIONAL | `features/git/components/commit-popover.tsx:126-129` (`<Checkbox>` per changed file). Chain: commit-popover.tsx → branch-section.tsx → git-panel.tsx → sidebar-carousel.tsx → ide-shell.tsx → routes/_shell.tsx. Gate: Git sidebar panel selected, repo has changed files, user opens "Commit changes" popover. |
| command | `web/src/components/ui/command.tsx` (`Command`/`CommandDialog`/`CommandInput`/`CommandPanel`/`CommandList`/`CommandItem`/`CommandFooter`) | CONDITIONAL | Real importers: `components/layout/workspace-switcher.tsx` and `components/layout/context-pill.tsx:5,47-108` (a third apparent hit, `code-block-node.tsx`, imports `cmdk`'s `Command`, a different package — false positive, ruled out). Chain: context-pill.tsx → `ide-shell.tsx:150` (`{!hasNavScreen && <ContextPill />}`, false by default) → routes/_shell.tsx. Gate: user clicks "Switch workspace" in the Context Pill to open the `CommandDialog`. |
| crowbar-mark | `web/src/components/ui/crowbar-mark.tsx` (`CrowbarMark`) | CONDITIONAL | Sole real importer: `features/tabs/components/tab-bar-item.tsx:16,141` (two other grep hits are unrelated string matches on the `crowbar-markdown-editor` class name, ruled out). Chain: tab-bar-item.tsx → tab-bar.tsx → pane-container.tsx → … → ide-shell.tsx → routes/_shell.tsx (same spine as button/checkbox). Gate: the pane's active buffer has `type === 'newTab'` — mapping doc's own count was "0 in the resting IDE, 1 after clicking New tab." |
| crowbar-wordmark | `web/src/components/ui/crowbar-wordmark.tsx` (`CrowbarWordmark`) | LIVE | Chain: `routes/_shell.tsx:2,5` → `ide-shell.tsx:13` (`WorkspaceHost`) → `workspace-host.tsx:8,267` (`WorkspaceView`) → `workspace-view.tsx:9,130` (`WorkspaceLayoutRoot`) → `workspace-layout-root.tsx:1,7` (`SplitViewRoot`) → `split-view-root.tsx:10,55` (`PaneContainer`) → `pane-container.tsx:22,451,571` (`NewTabView`) → `new-tab-view.tsx:4,394` (`CrowbarWordmark`) — renders on the app's own new-tab/empty pane state. Two other call sites in `components/oobe/oobe-screen.tsx:347,387` are unreachable per `native/mapping/crowbar-wordmark.md` §3 (the `/oobe` route only redirects to when there are zero projects, which the live fixture never has). |
| detach-holder-modal | `web/src/components/layout/detach-holder-modal.tsx` (`DetachHolderModal`, built on `dialog.tsx`'s Dialog/DialogPopup/Header/Title/Description/Footer) | CONDITIONAL | `ide-shell.tsx:22,277` renders `<DetachHolderModal />` unconditionally, but the component itself (`detach-holder-modal.tsx:23`) is `if (!target) return null`. Gate: `useDetachModalStore((s) => s.target)` non-null, set only by `openDetach(...)` in `components/layout/placeholder-row-actions.tsx:14,28-35`, reachable from a placeholder workspace row where `workspace.heldByPath` is set (branch checked out/held by another worktree) — `placeholder-row-actions.tsx:51`, used by `workspace-tree-item.tsx` in the (live) sidebar tree. |
| dialog | `web/src/components/ui/dialog.tsx` (`DialogPopup`/`DialogContent` + Header/Title/Description/Footer — **not** `AppDialog`) | LIVE | Two of 11 call sites reachable per `native/mapping/dialog.md` §5: `components/projects/add-repository-modal.tsx` (← `project-home-row.tsx` ← `workspace-tree.tsx` ← `sidebar-carousel.tsx:4` ← `ide-shell.tsx:10`) and `components/projects/import-project-modal.tsx` (← `project-switcher-panel.tsx` ← `workspace-switcher.tsx` ← `context-pill.tsx` ← `ide-shell.tsx:9` → routes/_shell.tsx). `AppDialog` (7 call sites) is a distinct export bypassing DialogHeader/DialogTitle entirely — not what this surface ports. |
| dropdown | `web/src/components/ui/dropdown.tsx` (`Dropdown`) | LIVE | `Dropdown` imported at `features/file-explorer/file-explorer/components/file-explorer-tree.tsx:38`, the reachable "filter menu" (funnel icon, Files sidebar tab) per `native/mapping/dropdown.md` §7. Chain: re-exported by `features/file-explorer/components/file-explorer-tree.tsx` (compat shim) ← `sidebar-carousel.tsx:4` ← `ide-shell.tsx:10` → routes/_shell.tsx. |
| dropdown-menu | `web/src/components/ui/dropdown-menu.tsx` (`DropdownMenu`/`DropdownMenuTrigger`/`DropdownMenuContent`/`DropdownMenuItem`/`DropdownMenuSeparator`) | LIVE | No dedicated `native/mapping/dropdown-menu.md`; record lives in `native/MAPPING.md:35` (P2.1) — see contradiction note below. Rust doc (`surfaces/dropdown_menu.rs:4-7`) names the reachable fixture as the comment menu in `features/git/components/review-thread-item.tsx:152-187`. Chain: review-thread-item.tsx ← `use-review-annotations.tsx`/`review-code-view.tsx` ← `review-diff-tab.tsx` ← `commit-diff-pane.tsx`/`branch-review-pane.tsx`, lazily rendered from `pane-container.tsx:44-45,69-70,467,481` (already-live chain). Requires opening a branch-review/commit-diff pane with a review comment thread present. |
| file-tree-row | `web/src/features/file-explorer/file-explorer/components/file-explorer-tree-item.tsx` (`FileExplorerTreeItem`, rendered via `TreeRow`) | LIVE | Surface-name/doc-name mismatch is intentional: `native/mapping/file-tree-row.md` (P1.5/P3.12) covers this surface directly; `native/mapping/tree-row.md` (P3.30) is a separate, later finding that the standalone `ui/tree-row.tsx` primitive is *not* its own surface — fully covered by `file-tree-row` and `git-status-row` (two independent re-implementations of `TreeRow`'s `<button>` for two CSS cascades). `FileExplorerTreeItem` imported at `file-explorer-tree.tsx:51,1304` ← shim ← `sidebar-carousel.tsx` ← `ide-shell.tsx:10` → routes/_shell.tsx. |
| flicker-spinner | `web/src/components/ui/flicker-spinner.tsx` (`FlickerSpinner`) | CONDITIONAL | Rendered at `features/agent/components/agent-chat-pane.tsx:639-641`, only while `attachment.state === 'reviving'` (an agent chat's provider spawn in flight — set at lines 356, 466: switching an agent chat's provider or resuming a chat). `agent-chat-pane.tsx` renders via `pane-container.tsx` (live chain). Matches `native/mapping/flicker-spinner.md` §4. Two sibling call sites (`agent-chat-glyph.tsx`, `workspace-branch-icon.tsx`) are gated on an actual agent turn. |
| fps-overlay | `web/src/components/layout/fps-overlay.tsx` (`FpsOverlay`) | CONDITIONAL | `<FpsOverlay />` rendered unconditionally at `ide-shell.tsx:276`, but `FpsOverlay` (`fps-overlay.tsx:91-94`) reads `useSettingsStore((s) => s.settings.showFpsOverlay)` and returns `null` if false. Setting defined `features/settings/types/settings.ts:62`, defaulted `false` in `features/settings/config/default-settings.ts:70`, toggled via a `Switch` in `features/settings/components/tabs/developer-settings.tsx:81-97` (Settings → Developer tab). Ships in every build — matches `native/mapping/fps-overlay.md` §0. |

## Notes and contradictions (accumulated as batches land)

- **badge**: `isAgent: true` is reachable in real production use (an AI agent
  posting a review comment via the daemon API), but no button/action inside
  `web/src` itself ever sets it true — every write the web app makes is
  hardcoded `isAgent: false` (`use-review-annotations.tsx:208,237`). Classed
  CONDITIONAL rather than UNCERTAIN because the gate is real and named, but
  confirming a human ever actually observes it live would require the
  daemon/agent side, outside `web/src`.
- **card**: `native/mapping/card.md` §0's table of fallback-less
  `ErrorBoundary` sites lists only three (`ide-shell.tsx:152`, `:162`,
  `sidebar-carousel.tsx:130`) and **omits** `web/src/routes/__root.tsx:17` —
  an `<ErrorBoundary>` with no `fallback` prop wrapping the entire route
  `<Outlet>`, the single most directly root-reachable fallback-less boundary
  in the app. Doesn't change the verdict (card.md's central finding — no live
  call site *intentionally* produces a Card — still holds) but the doc
  under-counted the reachable gates by one, and the omitted one is the
  strongest of the four. **Reported as required by the brief.**
- **dropdown-menu**: no `native/mapping/dropdown-menu.md` exists; the record
  lives in `native/MAPPING.md:35` (P2.1). That doc itself notes
  `dropdown-menu` was never a strict-parity result. Not a contradiction, just
  a naming-convention gap (per-file mapping docs postdate P2.1).
- **tree-row.md vs file-tree-row.md**: not a contradiction — `tree-row.md`
  (P3.30) is a later finding that the standalone `tree-row.tsx` primitive
  needed no surface of its own, since `file-tree-row` and `git-status-row`
  already cover its only DOM output. Both docs agree.
