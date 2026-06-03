# Sidebar Toggle Button Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a button in the tab bar (left of ← → arrows) that hides/shows the sidebar panel, giving the content area full width when hidden.

**Architecture:** Four coordinated changes — extend the layout sidebar store with a `sidebarVisible` boolean, make `ResizablePanel` forward refs so it can be imperatively controlled, wire `IDEShell` to collapse/expand the panel when the store changes, and add the toggle button to the tab bar. The `SidebarSimple` icon (`PanelLeftClose`) is already imported in `tab-bar.tsx`. Size is restored automatically by `react-resizable-panels` when `.expand()` is called.

**Tech Stack:** React 18, Zustand (`createSelectors`), `react-resizable-panels` (`ImperativePanelHandle`), Phosphor Icons (`SidebarSimple`), Tailwind CSS, TypeScript, Vitest (`bun run test`), `bunx tsc --noEmit` for type checking.

---

### Task 1: Extend the layout sidebar store

**Files:**
- Modify: `web/src/features/layout/stores/sidebar-store.ts`
- Create: `web/src/__tests__/features/layout/stores/sidebar-store.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `web/src/__tests__/features/layout/stores/sidebar-store.test.ts`:

```typescript
import { useSidebarStore } from '@/features/layout/stores/sidebar-store'

beforeEach(() => {
  useSidebarStore.setState({ sidebarVisible: true })
})

it('defaults sidebarVisible to true', () => {
  expect(useSidebarStore.getState().sidebarVisible).toBe(true)
})

it('setSidebarVisible(false) sets sidebarVisible to false', () => {
  useSidebarStore.getState().setSidebarVisible(false)
  expect(useSidebarStore.getState().sidebarVisible).toBe(false)
})

it('setSidebarVisible(true) restores sidebarVisible', () => {
  useSidebarStore.getState().setSidebarVisible(false)
  useSidebarStore.getState().setSidebarVisible(true)
  expect(useSidebarStore.getState().sidebarVisible).toBe(true)
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && bun run test src/__tests__/features/layout/stores/sidebar-store.test.ts
```

Expected: 3 failures — `sidebarVisible` and `setSidebarVisible` do not exist yet.

- [ ] **Step 3: Implement the store change**

Replace the entire content of `web/src/features/layout/stores/sidebar-store.ts`:

```typescript
import { create } from "zustand";
import { createSelectors } from "@/utils/zustand-selectors";

interface SidebarState {
  activePath?: string;
  updateActivePath: (path: string) => void;
  sidebarVisible: boolean;
  setSidebarVisible: (visible: boolean) => void;
}

const useSidebarStoreBase = create<SidebarState>()((set) => ({
  activePath: undefined,
  updateActivePath: (path: string) => {
    set({ activePath: path });
  },
  sidebarVisible: true,
  setSidebarVisible: (visible: boolean) => {
    set({ sidebarVisible: visible });
  },
}));

export const useSidebarStore = createSelectors(useSidebarStoreBase);
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd web && bun run test src/__tests__/features/layout/stores/sidebar-store.test.ts
```

Expected: 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/layout/stores/sidebar-store.ts web/src/__tests__/features/layout/stores/sidebar-store.test.ts
git commit -m "feat: add sidebarVisible state to layout sidebar store"
```

---

### Task 2: Make ResizablePanel forward refs

**Files:**
- Modify: `web/src/components/ui/resizable.tsx`

The current `ResizablePanel` wrapper drops `ref` because it uses spread-props without `React.forwardRef`. We need the ref to call `.collapse()` / `.expand()` imperatively from `IDEShell`.

- [ ] **Step 1: Replace the file**

Replace the entire content of `web/src/components/ui/resizable.tsx`:

```tsx
"use client"

import * as React from "react"
import * as ResizablePrimitive from "react-resizable-panels"
import type { ImperativePanelHandle } from "react-resizable-panels"

import { cn } from "@/lib/utils"

function ResizablePanelGroup({
  className,
  ...props
}: ResizablePrimitive.GroupProps) {
  return (
    <ResizablePrimitive.Group
      data-slot="resizable-panel-group"
      className={cn(
        "flex h-full w-full aria-[orientation=vertical]:flex-col",
        className
      )}
      {...props}
    />
  )
}

const ResizablePanel = React.forwardRef<
  ImperativePanelHandle,
  ResizablePrimitive.PanelProps
>(function ResizablePanel({ ...props }, ref) {
  return (
    <ResizablePrimitive.Panel
      data-slot="resizable-panel"
      ref={ref}
      {...props}
    />
  )
})

function ResizableHandle({
  withHandle,
  className,
  ...props
}: ResizablePrimitive.SeparatorProps & {
  withHandle?: boolean
}) {
  return (
    <ResizablePrimitive.Separator
      data-slot="resizable-handle"
      className={cn(
        "relative flex w-px items-center justify-center ring-offset-background after:absolute after:inset-y-0 after:left-1/2 after:w-1 after:-translate-x-1/2 focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-hidden aria-[orientation=horizontal]:h-px aria-[orientation=horizontal]:w-full aria-[orientation=horizontal]:after:left-0 aria-[orientation=horizontal]:after:h-1 aria-[orientation=horizontal]:after:w-full aria-[orientation=horizontal]:after:translate-x-0 aria-[orientation=horizontal]:after:-translate-y-1/2 [&[aria-orientation=horizontal]>div]:rotate-90",
        className
      )}
      {...props}
    >
      {withHandle && (
        <div className="z-10 flex h-6 w-1 shrink-0 rounded-lg bg-border" />
      )}
    </ResizablePrimitive.Separator>
  )
}

export { ResizableHandle, ResizablePanel, ResizablePanelGroup }
```

- [ ] **Step 2: Type-check**

```bash
cd web && bunx tsc --noEmit 2>&1 | head -30
```

Expected: no new errors. Note any pre-existing errors so you can distinguish them from regressions.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/ui/resizable.tsx
git commit -m "fix: forward ref in ResizablePanel to expose ImperativePanelHandle"
```

---

### Task 3: Wire IDEShell to collapse/expand sidebar via store

**Files:**
- Modify: `web/src/components/layout/IDEShell.tsx`

- [ ] **Step 1: Update the React import**

In `IDEShell.tsx`, line 1 currently reads:
```tsx
import { useState } from 'react'
```

Change it to:
```tsx
import { useState, useRef, useEffect } from 'react'
```

- [ ] **Step 2: Add two new imports at the bottom of the import block**

After the last import line (`import { Toaster } from '@/components/ui/sonner'`), add:

```tsx
import type { ImperativePanelHandle } from 'react-resizable-panels'
import { useSidebarStore as useLayoutSidebarStore } from '@/features/layout/stores/sidebar-store'
```

The alias `useLayoutSidebarStore` avoids a name collision with the existing `useSidebarStore` import from `@/lib/store/sidebar`.

- [ ] **Step 3: Add ref, store reads, and effect inside the component**

In the `IDEShell` function body, after the existing line:
```tsx
const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)
```

Add:
```tsx
const sidebarPanelRef = useRef<ImperativePanelHandle>(null)
const sidebarVisible = useLayoutSidebarStore((s) => s.sidebarVisible)
const setSidebarVisible = useLayoutSidebarStore((s) => s.setSidebarVisible)

useEffect(() => {
  if (sidebarVisible) {
    sidebarPanelRef.current?.expand()
  } else {
    sidebarPanelRef.current?.collapse()
  }
}, [sidebarVisible])
```

- [ ] **Step 4: Add collapsible props and ref to the sidebar ResizablePanel**

Find (around line 47 of the original file):
```tsx
<ResizablePanel id="sidebar" defaultSize="20%" minSize="12%" maxSize="45%" className="flex flex-col overflow-hidden">
```

Replace with:
```tsx
<ResizablePanel
  id="sidebar"
  ref={sidebarPanelRef}
  defaultSize="20%"
  minSize="12%"
  maxSize="45%"
  collapsible
  collapsedSize={0}
  onCollapse={() => setSidebarVisible(false)}
  onExpand={() => setSidebarVisible(true)}
  className="flex flex-col overflow-hidden"
>
```

- [ ] **Step 5: Hide the ResizableHandle when sidebar is collapsed**

Find the panel group render (near the bottom of the JSX):
```tsx
{sidebarPosition === "right" ? (
  <>{contentPanel}<ResizableHandle />{sidebarPanel}</>
) : (
  <>{sidebarPanel}<ResizableHandle />{contentPanel}</>
)}
```

Replace with:
```tsx
{sidebarPosition === "right" ? (
  <>{contentPanel}{sidebarVisible && <ResizableHandle />}{sidebarPanel}</>
) : (
  <>{sidebarPanel}{sidebarVisible && <ResizableHandle />}{contentPanel}</>
)}
```

- [ ] **Step 6: Type-check**

```bash
cd web && bunx tsc --noEmit 2>&1 | head -30
```

Expected: no new errors.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/layout/IDEShell.tsx
git commit -m "feat: wire sidebar panel collapse to sidebarVisible store state"
```

---

### Task 4: Add toggle button to the tab bar

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar.tsx`

- [ ] **Step 1: Add store selectors**

`useSidebarStore` is already imported at line 73 and `useSettingsStore` at line 69. In the component body, after the existing line:
```tsx
const { updateActivePath } = useSidebarStore();
```

Add:
```tsx
const sidebarVisible = useSidebarStore((s) => s.sidebarVisible)
const setSidebarVisible = useSidebarStore((s) => s.setSidebarVisible)
const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
```

- [ ] **Step 2: Insert the toggle button before the back/forward group**

In the JSX, find the back/forward `div` (around line 723 in the original file):
```tsx
<div className="flex shrink-0 items-center gap-0.5">
  <Button
    type="button"
    onClick={handleJumpBack}
    disabled={!canGoBack}
    variant="ghost"
    className="h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground"
    tooltip="Go Back"
```

Insert this immediately before that `div`:
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
    <PanelLeftClose size={14} />
  </Button>
)}
```

`PanelLeftClose` is already imported (it is `SidebarSimple` from `@phosphor-icons/react`, aliased at line 40–41). `isBottomPane` is already defined at line 171. `cn` is already imported.

- [ ] **Step 3: Type-check**

```bash
cd web && bunx tsc --noEmit 2>&1 | head -30
```

Expected: no new errors.

- [ ] **Step 4: Run the full test suite**

```bash
cd web && bun run test
```

Expected: all tests pass, including the 3 new sidebar-store tests.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/tabs/components/tab-bar.tsx
git commit -m "feat: add sidebar toggle button to tab bar"
```

---

### Task 5: Manual verification

- [ ] **Step 1: Start the dev server**

```bash
cd web && bun run dev
```

- [ ] **Step 2: Verify the happy path**

1. Open the app. Sidebar is visible (default).
2. The toggle button (⊡ icon) appears in the tab bar, to the left of the ← → arrows.
3. Click toggle. Sidebar fully hides; content expands to full width; resize handle disappears.
4. Click toggle again. Sidebar reappears at its previous width; resize handle is back.

- [ ] **Step 3: Verify size is preserved**

1. Drag the sidebar to ~35% width.
2. Click toggle to hide. Click toggle to show.
3. Sidebar should restore to ~35%, not snap back to the 20% default.

- [ ] **Step 4: Verify sidebar-right setting**

1. Open Settings → change sidebar position to "right".
2. The toggle icon should be mirrored (flipped horizontally) to point toward the right sidebar.
3. Toggle works in both directions.

- [ ] **Step 5: Verify bottom pane exclusion**

Open a terminal tab (bottom pane). The toggle button should NOT appear in the bottom pane's tab bar.

- [ ] **Step 6: Commit any fixups**

If small fixes were needed:
```bash
git add -p
git commit -m "fix: sidebar toggle button edge case"
```
