import { EditorHostRegistry } from '@/features/panes/components/editor-host-registry'
import { MobileChatStage } from '@/features/panes/components/mobile-chat-stage'

export function WorkspaceLayoutRoot() {
  return (
    // Takes what the view-tab strip leaves; min-h-0 keeps tall pane content
    // from holding it open past the window's bottom edge.
    <div className="min-h-0 flex-1">
      <MobileChatStage />
      {/* Sibling of the pane tree so rebuilding a pane never reaches the retained
          editor widgets (editor-host-registry.tsx); on the phone it portals into
          the IDE screen's editor tabs. */}
      <EditorHostRegistry />
    </div>
  )
}
