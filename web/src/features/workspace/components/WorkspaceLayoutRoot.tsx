import { SplitViewRoot } from '@/features/panes/components/split-view-root'
import { BottomPane } from './bottom-pane'
import { useUIState } from '@/features/window/stores/ui-state-store'

export function WorkspaceLayoutRoot() {
  const isBottomPaneVisible = useUIState(s => s.isBottomPaneVisible)

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="min-h-0 flex-1 overflow-hidden">
        <SplitViewRoot />
      </div>
      {isBottomPaneVisible && <BottomPane />}
    </div>
  )
}
