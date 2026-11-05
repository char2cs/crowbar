import { Outlet } from '@tanstack/react-router'
import { WorkspaceHost } from '@/features/workspace/components/workspace-host'
import { ErrorBoundary } from '@/components/error-boundary'
import { ViewTabsRow } from '@/features/tabs/components/view-tabs-row'
import { getKnownHomeWorkspaceIds } from '@/features/workspace/lib/home-workspace-resolver'

// `IDEShell`'s content column (WorkspaceHost plus the Outlet), split out because it needs only the
// resolved workspace-id sets, none of the shell's sidebar or route state.
export function IdeShellWorkspaceContent({
  effectiveActiveWorkspaceId,
  paneWsIds,
  viewWsIds,
  showViewTabs = true,
}: {
  effectiveActiveWorkspaceId: string | null
  paneWsIds: string[]
  viewWsIds: string[]
  /** Off on the phone, which shows one chat at a time and lists the rest in its chat list. */
  showViewTabs?: boolean
}) {
  return (
    // Each pane is its own drop target (PaneContainer, §8.1); this column is not one.
    <div className="relative z-[1] flex h-full min-w-0 flex-col bg-transparent">
      {showViewTabs && <ViewTabsRow />}
      <ErrorBoundary>
        {/* Mounted for the whole session, home route included, so retained workspaces stay warm. A
            workspace stays mounted while home (homeWsIds), a pane (paneWsIds) or Recents (viewWsIds)
            names it. The Outlet stays mounted so route-level guards keep running. */}
        <WorkspaceHost
          activeWsId={effectiveActiveWorkspaceId}
          homeWsIds={getKnownHomeWorkspaceIds()}
          paneWsIds={paneWsIds}
          viewWsIds={viewWsIds}
        />
        <Outlet />
      </ErrorBoundary>
    </div>
  )
}
