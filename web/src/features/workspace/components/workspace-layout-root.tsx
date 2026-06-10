import { SplitViewRoot } from '@/features/panes/components/split-view-root'

export function WorkspaceLayoutRoot() {
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="min-h-0 flex-1 overflow-hidden">
        <SplitViewRoot />
      </div>
    </div>
  )
}
