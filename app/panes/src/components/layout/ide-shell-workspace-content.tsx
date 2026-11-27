import { Outlet } from '@tanstack/react-router'
import { WorkspaceHost } from '@/features/workspace/components/workspace-host'
import { ErrorBoundary } from '@/components/error-boundary'
import { ViewTabsRow } from '@/features/tabs/components/view-tabs-row'
import { getKnownHomeWorkspaceIds } from '@/features/workspace/lib/home-workspace-resolver'

export function IdeShellWorkspaceContent({
  effectiveActiveWorkspaceId,
  paneWsIds,
  viewWsIds,
  showViewTabs = true,
}: {
  effectiveActiveWorkspaceId: string | null
  paneWsIds: string[]
  viewWsIds: string[]
  showViewTabs?: boolean
}) {
  return (
    <div className="relative z-[1] flex h-full min-w-0 flex-col bg-transparent">
      {showViewTabs && <ViewTabsRow />}
      <ErrorBoundary>
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
