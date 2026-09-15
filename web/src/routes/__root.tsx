// web/src/routes/__root.tsx
import { createRootRoute, Outlet } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { AppSyncProvider } from '@/components/app-sync-provider'
import { AnchoredToastProvider } from '@/components/ui/toast'
import { DaemonHealthListener } from '@/features/window/components/daemon-health-listener'

// Note: there is deliberately NO global viewport for `toastManager` here.
// SidebarToastOverlay (inside IDEShell) is the one and only viewport for it —
// a second Toast.Provider on the same manager gives each provider its own copy
// of the toast list, so every toast renders twice whenever both are mounted.
// The anchored manager is separate and keeps its own root-level viewport.
//
// Boot hydration (main.tsx's `hydrateCriticalStores`) runs and resolves
// BEFORE this ever mounts — there is no gate here. See that function's own
// doc for why: a `WorkspaceView`/`EditorSurface` mounted against an
// empty/default pane layout, only to have it replaced a frame later by the
// real persisted one, is a real crash, not just a flash.
function RootComponent() {
  return (
    <ErrorBoundary>
      <AppSyncProvider>
        <AnchoredToastProvider>
          <DaemonHealthListener />
          <Outlet />
        </AnchoredToastProvider>
      </AppSyncProvider>
    </ErrorBoundary>
  )
}

export const Route = createRootRoute({
  component: RootComponent,
})
