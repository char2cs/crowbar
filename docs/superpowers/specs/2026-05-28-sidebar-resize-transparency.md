# Sidebar Resize + Transparency Fix

**Date:** 2026-05-28  
**Scope:** `web/src/components/layout/IDEShell.tsx` only

## Problem

1. The shadcn `Sidebar` component renders an inner `<div data-slot="sidebar-inner">` with `bg-sidebar` hardcoded (`oklch(0.18 0 0)` in dark mode), making the sidebar opaque when the window background should show through.
2. The shadcn `Sidebar` has a fixed `--sidebar-width: 16rem` with no resize capability.

## Fix 1 — Transparent sidebar background

Add a Tailwind arbitrary-child selector to `<Sidebar>`'s `className` prop that overrides the inner div's background:

```tsx
<Sidebar
  side={sidebarPosition}
  collapsible="offcanvas"
  className="[&>[data-slot=sidebar-inner]]:bg-transparent"
>
```

Targets `data-slot="sidebar-inner"` directly inside `data-slot="sidebar-container"` (the element `className` is applied to). No global CSS variable changes.

## Fix 2 — Resizable sidebar with localStorage persistence

### Width state

```tsx
const [sidebarWidth, setSidebarWidth] = useState(
  () => parseInt(localStorage.getItem('sidebar-width') ?? '256', 10)
)
```

Default 256px (= 16rem, matching current shadcn default). Initialized lazily from `localStorage`.

### CSS variable override

```tsx
<SidebarProvider
  className="h-screen overflow-hidden bg-transparent text-foreground"
  style={{ "--sidebar-width": `${sidebarWidth}px` } as React.CSSProperties}
>
```

SidebarProvider merges `style` after its own hardcoded `--sidebar-width: 16rem`, so our value wins.

### Drag handle

Wrap all `<Sidebar>` children in a positioning context div so the absolute drag handle is contained:

```tsx
<div className="relative flex h-full flex-col overflow-hidden">
  <div
    className={cn(
      "absolute inset-y-0 z-50 w-1 cursor-col-resize opacity-0 hover:opacity-100 hover:bg-border transition-opacity",
      sidebarPosition === 'right' ? 'left-0' : 'right-0'
    )}
    onMouseDown={handleResizeDragStart}
  />
  {/* titlebar strip, SidebarHeader, SidebarTabs … */}
</div>
```

- Width 4px (`w-1`), invisible until hovered (`opacity-0 hover:opacity-100`)
- Right edge for left sidebar, left edge for right sidebar
- `cursor-col-resize` sets the ↔ cursor

### Drag handler

```tsx
function handleResizeDragStart(e: React.MouseEvent) {
  e.preventDefault()
  const startX = e.clientX
  const startWidth = sidebarWidth

  function onMouseMove(e: MouseEvent) {
    const delta = sidebarPosition === 'left' ? e.clientX - startX : startX - e.clientX
    const next = Math.min(640, Math.max(192, startWidth + delta))
    setSidebarWidth(next)
    localStorage.setItem('sidebar-width', String(next))
  }

  function onMouseUp() {
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
  }

  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}
```

- Document-level listeners so drag continues when pointer leaves the handle
- Width clamped to `[192, 640]` px (12rem – 40rem)
- Persists to `localStorage` on every move event

## Constraints

- No new files — all changes in `IDEShell.tsx`
- No new dependencies
- Width is not synced with SidebarProvider's collapse state — when toggled closed, the gap div goes to 0 regardless of width; on reopen, it restores to the last dragged width (correct behavior from CSS)
- `handleResizeDragStart` must be defined before the JSX return (can be a regular function inside the component)
