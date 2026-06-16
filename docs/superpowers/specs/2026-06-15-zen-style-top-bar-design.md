# Zen-style Sidebar Top Bar — Design

Date: 2026-06-15
Status: Approved (spec + plan being produced together at user request)

## Problem

Crowbar's sidebar header (`SidebarProjectHeader`) currently holds only a
settings gear. The browser-style navigation controls (sidebar-toggle +
back/forward) live in the **editor tab bar** (`tab-navigation-buttons.tsx`).
We want to refactor the sidebar header into a Zen-browser-style top bar:
a sidebar-toggle on the leading edge and a back / forward / settings cluster on
the trailing edge, mirrored correctly when the sidebar is on the right.

## Goals

- Sidebar header becomes the Zen-style top bar.
- Back/forward reuse the **existing** editor jump navigation
  (`useJumpNavigation`: editor location history + active webviewer nav).
- The settings gear takes the rightmost ("reload") slot of the cluster.
- Layout mirrors based on `sidebarPosition` ('left' | 'right').
- Traffic-light spacing is preserved on whichever side is top-left.
- Back/forward are removed from the tab bar; the tab bar keeps **only** the
  sidebar-toggle (so a collapsed sidebar can still be reopened).

## Non-goals

- No URL/address bar (Zen has one below its toolbar; Crowbar keeps the
  `ContextPill` + tabs/tree below the header unchanged).
- No Rust / `tauri.conf.json` changes. Traffic lights stay physically top-left
  (macOS); we only reserve space around them in the layout.
- No new keyboard shortcuts (existing jump shortcuts, if any, are untouched).
- No change to NavStack pushed-screen headers (project switcher / repo
  settings) — those still render their own back+title header.

## Existing building blocks (reused as-is)

- `useJumpNavigation` (`features/tabs/hooks/use-jump-navigation.ts`) — returns
  `{ canGoBack, canGoForward, handleJumpBack, handleJumpForward }` given
  `{ usesWebViewerNavigation, activeWebViewerNavigation }`.
- `useWebViewerNavigationStore` — `navigationByBufferId[bufferId]` →
  `{ canGoBack, canGoForward, goBack, goForward }`.
- `useSidebar()` (`@/components/ui/sidebar`) — `{ open, toggleSidebar }`.
- `useUIState().openSettingsDialog()` — opens the settings dialog.
- Workspace store: `activePaneId`, `panes[id].activeBufferId`, `buffers`.
- Icons: `SidebarSimple` (toggle), `ArrowLeft` / `ArrowRight`
  (`@phosphor-icons/react`) — matching the current tab-bar buttons; `GearSix`
  for settings.

## Design

### 1. New hook: `useActiveWebViewerNavigation`

File: `web/src/features/tabs/hooks/use-active-webviewer-navigation.ts`

Derives jump-nav inputs from the **globally active** pane (the header is a
single global instance, unlike the per-pane tab bar):

```ts
export function useActiveWebViewerNavigation(): {
  usesWebViewerNavigation: boolean
  activeWebViewerNavigation: WebViewerNavigation | undefined
}
```

- Read the active buffer: `activePaneId` → `panes[activePaneId].activeBufferId`
  → matching entry in `buffers`.
- `usesWebViewerNavigation = activeBuffer?.type === 'webViewer'`.
- `activeWebViewerNavigation = usesWebViewerNavigation`
  ? `useWebViewerNavigationStore((s) => s.navigationByBufferId[activeBuffer.id])`
  : `undefined`.
- Narrow selectors only; no `getState()` in render.

The tab bar may optionally adopt this hook later, but this refactor leaves the
tab bar's own derivation intact except for removing the back/forward UI.

### 2. `SidebarProjectHeader` (rewrite)

Composition (leading → trailing), driven by `isRight = sidebarPosition === 'right'`:

- **Left sidebar:** `[52px traffic-light spacer] [toggle] [flex spacer] [back] [forward] [settings]`
- **Right sidebar:** `[settings] [back] [forward] [flex spacer] [toggle]` (no spacer — traffic lights sit over the main content area instead)

Details:
- Container: `flex w-full flex-shrink-0 items-center px-3`, height `h-[44px]`
  (mac) / `h-[34px]`, `data-tauri-drag-region`. Use `flex-row-reverse` (or
  conditional ordering) for the right-sidebar mirror.
- **Toggle**: `useSidebar().toggleSidebar`; `SidebarSimple` icon; apply
  `scale-x-[-1]` when `isRight` (matches the existing tab-bar toggle); tooltip
  "Hide/Show Sidebar"; `aria-label` accordingly.
- **Back/forward**: wired to `useJumpNavigation(useActiveWebViewerNavigation())`;
  `disabled` on `!canGoBack` / `!canGoForward`; `aria-label`s "Go back to
  previous location" / "Go forward to next location".
- **Settings**: `GearSix`, `openSettingsDialog()`, `aria-label="Settings"`.
- Buttons are non-draggable (they sit inside the drag region but must remain
  clickable — interactive elements already exclude themselves from
  `data-tauri-drag-region`).

The header continues to render only on the root sidebar view (when
`!hasNavScreen`), exactly as today.

### 3. Tab bar changes

- `tab-navigation-buttons.tsx`: remove the back/forward `<Button>`s and the
  `canGoBack` / `canGoForward` / `onJumpBack` / `onJumpForward` props. Keep the
  sidebar-toggle block. (Consider renaming the component to
  `TabSidebarToggle`, but renaming is optional and can be deferred to avoid
  churn.)
- `tab-bar.tsx`: stop calling `useJumpNavigation` and stop computing
  `activeWebViewerNavigation` / `usesWebViewerNavigation` solely for these
  buttons; stop passing the removed props. (Leave any other usage intact — verify
  none remains before deleting the derivations.)

### 4. Layout / mirroring & traffic lights

- The 52px traffic-light spacer renders only when `IS_MAC && !isRight` (in the
  sidebar header) — unchanged rule, now sitting before the toggle.
- No traffic-light repositioning; `tauri.conf.json` untouched.

## Component boundaries

- `useActiveWebViewerNavigation` — single responsibility: expose the active
  pane's webviewer nav; independently testable with seeded stores.
- `SidebarProjectHeader` — layout + wiring only; delegates behavior to the hooks
  above.
- Tab-bar components shrink (lose a responsibility) — net simplification.

## Testing

- `use-active-webviewer-navigation.test.ts` — active buffer is a webViewer →
  returns its nav object + `usesWebViewerNavigation: true`; active buffer is a
  file → `usesWebViewerNavigation: false`, nav `undefined`; no active pane →
  safe defaults.
- `sidebar-project-header.test.tsx` — renders toggle + back + forward + settings;
  toggle calls `toggleSidebar`; settings calls `openSettingsDialog`; back/forward
  disabled when `canGoBack/Forward` are false and call the jump handlers when
  enabled; right-sidebar mirror renders (e.g. ordering / `scale-x-[-1]` on
  toggle). Mock `@/components/ui/sidebar`, the jump-nav inputs, and the UI store
  as needed.
- Update `ide-shell.test.tsx` only if the header stub interaction changes (it
  currently stubs `SidebarProjectHeader`, so likely unaffected).
- Update/trim any `tab-navigation-buttons` / `tab-bar` tests that referenced the
  removed back/forward props.

## Conventions

- Kebab-case files, PascalCase exports.
- Narrow Zustand selectors in render; `getState()` only in handlers.
- No store→component imports; CSS-variable tokens / `@/components/ui/*` only.
- `@/` imports in tests.

## Decisions (resolved)

- Back/forward map to the existing editor jump navigation. (Confirmed.)
- Nav + settings live in the header; back/forward removed from the tab bar; the
  tab bar keeps only the sidebar-toggle. (Confirmed.)
- Layout matches the Zen reference and mirrors for a right-side sidebar.
  (Confirmed.)
