// web/src/routes/__root.tsx
import { createRootRoute, Outlet } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { AppSyncProvider } from '@/components/app-sync-provider'
import { AnchoredToastProvider } from '@/components/ui/toast'
import { DaemonHaltGate } from '@/features/window/components/daemon-halt-gate'
import { DaemonHealthListener } from '@/features/window/components/daemon-health-listener'
import { ConnectionIndicator } from '@/components/layout/connection-indicator'
import { OperationsWatcher } from '@/components/layout/operations-watcher'

// Note: there is deliberately NO global viewport for `toastManager` here.
// SidebarToastOverlay is its one viewport, mounted by IDEShell or the OOBE (one
// route at a time): a second Toast.Provider on the same manager renders every
// toast twice. The watchers that raise toasts live here, so no route misses one.
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
          <ConnectionIndicator />
          <OperationsWatcher />
          <DaemonHaltGate>
            <Outlet />
          </DaemonHaltGate>
        </AnchoredToastProvider>
      </AppSyncProvider>
    </ErrorBoundary>
  )
}

export const Route = createRootRoute({
  component: RootComponent,
})
