import { EditorHostRegistry } from '@/features/panes/components/editor-host-registry'
import { MobileChatStage } from '@/features/panes/components/mobile-chat-stage'
import { SplitViewRoot } from '@/features/panes/components/split-view-root'
import { IS_MOBILE } from '@/utils/platform'

export function WorkspaceLayoutRoot() {
  return (
    <div className="min-h-0 flex-1">
      {IS_MOBILE ? <MobileChatStage /> : <SplitViewRoot />}
      <EditorHostRegistry />
    </div>
  )
}
