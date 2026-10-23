import { EditorHostRegistry } from '@/features/panes/components/editor-host-registry'
import { SplitViewRoot } from '@/features/panes/components/split-view-root'

export function WorkspaceLayoutRoot() {
  return (
    // Takes what the view-tab strip leaves; min-h-0 keeps tall pane content
    // from holding it open past the window's bottom edge.
    <div className="min-h-0 flex-1">
      <SplitViewRoot />
      {/* Sibling of SplitViewRoot, not inside it: a pane's own React subtree
          is free to be torn down and rebuilt by a tab switch, a split, or
          fullscreen without that ever reaching the retained editor widgets
          this owns — see editor-host-registry.tsx's own doc. */}
      <EditorHostRegistry />
    </div>
  )
}
