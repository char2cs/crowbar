import { SplitViewRoot } from '@/features/panes/components/split-view-root'

export function WorkspaceLayoutRoot() {
  return (
    <div className="flex h-full flex-col">
      <div className="min-h-0 flex-1">
        <SplitViewRoot />
      </div>
    </div>
  )
}
