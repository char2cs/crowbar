# Sidebar Toast Overlay

## Goal
Move toast rendering from a global fixed-position overlay to the bottom of the sidebar panel.
Toasts must appear as an **absolute-positioned overlay** inside the sidebar — they must NEVER
displace sidebar content or change its layout. When the sidebar is collapsed, toasts fall back
to a fixed-position overlay anchored at the same horizontal edge (left or right) as the sidebar.

## Global Constraints
- Never hardcode colors — always use CSS variable tokens
- Always use `@/components/ui/*` and CSS variable tokens; never raw library primitives at call sites
- Component files use kebab-case filenames; exported React components are PascalCase
- Stores: narrow selectors only (`useStore((s) => s.field)`); `getState()` only in effects/handlers
- Stores must not import from `components/` — lib/ shims for that
- Toasts must NEVER displace sidebar content — they overlay it via `position: absolute`
- TypeScript must compile clean (`pnpm --filter web tsc --noEmit`)
- No new npm dependencies
- The existing `toast.*` call-site API (toast-store.ts) remains unchanged

## Context

### Toast architecture
- `toastManager` — singleton created in `web/src/components/ui/toast.tsx`, re-exported from `web/src/lib/toast-manager.ts`
- `<ToastProvider>` in `web/src/routes/__root.tsx` — wraps the app; currently always renders `<Toasts>` (fixed-position portal to body, bottom-right). Must continue to work for non-IDE routes (OOBE, onboarding).
- `toast.*` calls push to `toastManager`
- `Toast.useToastManager()` reads toasts from the nearest `Toast.Provider` context in the tree

### IDEShell layout
- `web/src/components/layout/ide-shell.tsx`
- `sidebarContent` — a `div.flex.h-full.flex-col.overflow-hidden.bg-transparent.select-none` containing project header, tab bar, carousel
- `sidebarOpen: boolean` — local useState in IDEShell, reflects whether the ResizablePanel is expanded
- `sidebarPosition: 'left' | 'right'` — from `useSettingsStore((s) => s.settings.sidebarPosition)` (already imported)
- The sidebar ResizablePanel has `collapsible collapsedSize={0}` — at 0 width when collapsed

### UIState store
- `web/src/features/window/stores/ui-state-store.ts` — Zustand store; already has `isSettingsOpen`, `setIsSettingsDialogVisible`, etc.

---

## Task 1: Add suppression plumbing — `ideShellMounted` flag + `ToastProvider` `suppressToasts` prop

**Scope:** `web/src/features/window/stores/ui-state-store.ts`, `web/src/components/ui/toast.tsx`, `web/src/routes/__root.tsx`

### Steps

1. **Add `ideShellMounted` to `useUIState`** in `web/src/features/window/stores/ui-state-store.ts`:
   - Add to the `UIState` interface:
     ```ts
     ideShellMounted: boolean
     setIdeShellMounted: (v: boolean) => void
     ```
   - Add to the store initializer:
     ```ts
     ideShellMounted: false,
     setIdeShellMounted: (v) => set({ ideShellMounted: v }),
     ```

2. **Add `suppressToasts` prop to `ToastProvider`** in `web/src/components/ui/toast.tsx`:
   - Extend `ToastProviderProps`:
     ```ts
     export interface ToastProviderProps extends Toast.Provider.Props {
       position?: ToastPosition
       portalProps?: React.ComponentProps<typeof Toast.Portal>
       suppressToasts?: boolean
     }
     ```
   - Update `ToastProvider` to read the new prop and skip rendering `<Toasts>` when true:
     ```tsx
     export function ToastProvider({
       children,
       position = "bottom-right",
       portalProps,
       suppressToasts,
       ...props
     }: ToastProviderProps): React.ReactElement {
       return (
         <Toast.Provider toastManager={toastManager} {...props}>
           {children}
           {!suppressToasts && <Toasts portalProps={portalProps} position={position} />}
         </Toast.Provider>
       )
     }
     ```

3. **Wire into `__root.tsx`** — read `ideShellMounted` and pass `suppressToasts`:
   ```tsx
   import { useUIState } from '@/features/window/stores/ui-state-store'

   function RootComponent() {
     const ideShellMounted = useUIState((s) => s.ideShellMounted)
     return (
       <HydrationGate>
         <ErrorBoundary>
           <AppSyncProvider>
             <ToastProvider position="bottom-right" suppressToasts={ideShellMounted}>
               <AnchoredToastProvider>
                 <Outlet />
               </AnchoredToastProvider>
             </ToastProvider>
           </AppSyncProvider>
         </ErrorBoundary>
       </HydrationGate>
     )
   }
   ```

4. **Run `pnpm --filter web tsc --noEmit`** from `web/` — fix any TypeScript errors.

5. **Commit** with message: `feat(toast): add suppressToasts prop + ideShellMounted state`

### Notes
- Narrow selector `useUIState((s) => s.ideShellMounted)` triggers re-render only when this boolean changes
- When IDEShell is not mounted (OOBE, onboarding), `ideShellMounted` is false and root renders the fixed fallback as before

---

## Task 2: Create `SidebarToastOverlay` and wire into IDEShell

**Scope:** `web/src/components/layout/sidebar-toast-overlay.tsx` (new file), `web/src/components/layout/ide-shell.tsx`

Depends on Task 1: the `useUIState.getState().setIdeShellMounted(true/false)` calls live here.

### The overlay component

Create **`web/src/components/layout/sidebar-toast-overlay.tsx`**.

The component reads toasts from the `Toast.Provider` context already provided by the root `<ToastProvider>` (via `Toast.useToastManager()`). It has two rendering modes based on the `sidebarOpen` prop:

**Mode A — sidebar open:**
- Renders WITHOUT `Toast.Portal` (directly in the sidebar DOM — no body portal)
- Uses `Toast.Viewport` as the wrapper with `position: absolute; inset-x-0; bottom-0; z-50; pointer-events-none`
- Individual `Toast.Root` elements use `relative` positioning (flat list, NO stacking/absolute children)
- Layout is `flex flex-col-reverse gap-2 p-2` so newest toast appears at the bottom edge
- Full sidebar width (`w-full`, no `max-w-*`)
- `pointer-events-none` on container, `pointer-events-auto` on each Toast.Root

**Mode B — sidebar closed:**
- Renders via `Toast.Portal` (portals to body, exits the collapsed panel)
- Uses `Toast.Viewport` with `position: fixed; bottom-4; z-[var(--z-overlay,60)]; flex flex-col gap-2; w-72`
- `left-4` when `sidebarSide === 'left'`, `right-4` when `'right'`
- Same `Toast.Root` styling as Mode A

**Toast.Root styling (both modes):**
```tsx
className="pointer-events-auto relative flex w-full items-center justify-between gap-1.5 overflow-hidden rounded-lg border bg-popover px-3.5 py-3 text-sm text-popover-foreground shadow-lg/5 not-dark:bg-clip-padding transition-opacity duration-200 data-starting-style:opacity-0 data-ending-style:opacity-0"
```

**Toast content (same structure as `toast.tsx`):**
- Icon from `TOAST_ICONS` lookup on `toast.type`
- `Toast.Title` + `Toast.Description` in a flex column
- `Toast.Action` when `toast.actionProps` is present — pass `onClick={toast.actionProps.onClick}`
- `Toast.Close` button with an `X` icon (use lucide `X`)
  ```tsx
  <Toast.Close className="rounded p-0.5 opacity-50 hover:opacity-100 hover:bg-muted transition-opacity">
    <X className="h-3.5 w-3.5" />
  </Toast.Close>
  ```

**Limit:** In Mode A (sidebar open), show at most 3 toasts: `toasts.slice(0, 3)`. In Mode B show all.

**Swipe direction:** `sidebarSide === 'left' ? ['left', 'down'] : ['right', 'down']`

**Props interface:**
```ts
interface SidebarToastOverlayProps {
  sidebarOpen: boolean
  sidebarSide: 'left' | 'right'
}
```

### Wiring into IDEShell

In **`web/src/components/layout/ide-shell.tsx`**:

1. **Mount/unmount `ideShellMounted`** — add this effect inside `IDEShell`:
   ```ts
   useEffect(() => {
     useUIState.getState().setIdeShellMounted(true)
     return () => useUIState.getState().setIdeShellMounted(false)
   }, [])
   ```

2. **Add `relative` to `sidebarContent` outer div** — the overlay needs a positioned ancestor:
   ```tsx
   // Before:
   <div className="flex h-full flex-col overflow-hidden bg-transparent select-none">
   // After:
   <div className="relative flex h-full flex-col overflow-hidden bg-transparent select-none">
   ```

3. **Add `<SidebarToastOverlay>` as the last child of `sidebarContent`**:
   ```tsx
   <SidebarToastOverlay
     sidebarOpen={sidebarOpen}
     sidebarSide={sidebarPosition ?? 'left'}
   />
   ```
   Note: `sidebarPosition` from `useSettingsStore` may be `'left' | 'right'` — `?? 'left'` handles undefined.

4. **Import** `SidebarToastOverlay` from `'./sidebar-toast-overlay'` and `useUIState` is already imported.

5. **Run `pnpm --filter web tsc --noEmit`** — fix any TypeScript errors.

6. **Commit** with message: `feat(toast): SidebarToastOverlay — absolute in sidebar, fixed fallback when collapsed`

### Notes
- `relative` on the sidebarContent outer div is load-bearing for the `absolute inset-x-0 bottom-0` overlay
- `overflow-hidden` is already on the outer div; toasts are bounded by `inset-x-0` so they don't overflow horizontally; vertical overflow is safe (toasts grow upward within the div's height)
- `useUIState.getState()` in the effect body follows CLAUDE.md store pattern (getState only in effects)
- Both modes use the same icon lookup and content structure — extract a shared `SidebarToastItem` helper inside the file
