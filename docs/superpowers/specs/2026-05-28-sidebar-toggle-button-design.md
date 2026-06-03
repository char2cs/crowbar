# Sidebar Toggle Button

**Date:** 2026-05-28
**Branch:** enhancement/design-language

## Summary

Add a toggle button to the tab bar (next to the ← → navigation arrows) that collapses the sidebar panel entirely, giving the content area full width. Clicking again restores the sidebar to its previous size. The sidebar itself (ResizablePanel) is kept as-is; the inner components are not replaced with shadcn primitives in this change.

## Behavior

- Button lives in the tab bar, to the left of the ← → back/forward group
- Clicking hides the sidebar (collapses to 0px); content takes full width
- Clicking again restores the sidebar to whatever width it was at before collapse (`react-resizable-panels` remembers pre-collapse size internally; `.expand()` restores it without extra state)
- Button is shown on all non-bottom panes (`!isBottomPane`)
- When sidebar is collapsed, the resize handle is hidden to avoid a phantom drag target
- When `sidebarPosition === 'right'`, the icon is mirrored horizontally

## Files Changed

### 1. `web/src/features/layout/stores/sidebar-store.ts`

Add to `SidebarState`:
- `sidebarVisible: boolean` — default `true`
- `setSidebarVisible: (visible: boolean) => void`

No other changes to this file.

### 2. `web/src/components/ui/resizable.tsx`

`ResizablePanel` does not currently forward refs. Wrap it with `React.forwardRef<ImperativePanelHandle, ResizablePrimitive.PanelProps>` so the caller can receive the imperative handle needed to call `.collapse()` / `.expand()` programmatically.

Import `ImperativePanelHandle` from `react-resizable-panels`.

### 3. `web/src/components/layout/IDEShell.tsx`

- Import `useRef` (already present), `ImperativePanelHandle` from `react-resizable-panels`
- Import `useSidebarStore` from `@/features/layout/stores/sidebar-store`
- Create `sidebarPanelRef = useRef<ImperativePanelHandle>(null)`
- Read `sidebarVisible` and `setSidebarVisible` from the layout sidebar store
- Add to the sidebar `ResizablePanel`:
  - `ref={sidebarPanelRef}`
  - `collapsible={true}`
  - `collapsedSize={0}`
  - `onCollapse={() => setSidebarVisible(false)}`
  - `onExpand={() => setSidebarVisible(true)}`
- Add `useEffect` that calls `sidebarPanelRef.current?.collapse()` or `.expand()` when `sidebarVisible` changes
- Conditionally render `ResizableHandle` only when `sidebarVisible` is true — prevents a phantom drag target at the 0px edge

### 4. `web/src/features/tabs/components/tab-bar.tsx`

- `SidebarSimple` is already imported (aliased as `PanelLeftClose`) — reuse it
- Read `sidebarVisible` from layout sidebar store (store already imported at line 73)
- Read `sidebarPosition` from settings store (already imported at line 69)
- Add `setSidebarVisible` from the store
- Add a `Button` before the back/forward `div` (line 723), only when `!isBottomPane`:

```tsx
{!isBottomPane && (
  <Button
    type="button"
    onClick={() => setSidebarVisible(!sidebarVisible)}
    variant="ghost"
    compact
    className={cn(
      "h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground",
      sidebarPosition === 'right' && "scale-x-[-1]",
    )}
    tooltip={sidebarVisible ? "Hide Sidebar" : "Show Sidebar"}
    tooltipSide="bottom"
    aria-label={sidebarVisible ? "Hide sidebar" : "Show sidebar"}
  >
    <PanelLeftClose />
  </Button>
)}
```

Place the button immediately before the existing back/forward `div`, inside the outer tab bar `div`.

## Out of Scope

- Inner sidebar components are not replaced with shadcn primitives in this change
- No keyboard shortcut (Cmd+B or similar) — button only
- No animation on collapse/expand (ResizablePanel handles transition internally)
- Icon mirroring for right sidebar is a CSS-only change (no logic change)
