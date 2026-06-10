import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { ContextMenu } from '@/components/ui/context-menu'
import { IS_MAC } from '@/utils/platform'
import {
  buildEditorContextMenuItems,
  type EditorContextMenuHandlers,
} from './editor-context-menu-items'

interface EditorContextMenuProps extends EditorContextMenuHandlers {
  isOpen: boolean
  position: { x: number; y: number }
  onClose: () => void
}

const EditorContextMenu = ({ isOpen, position, onClose, ...handlers }: EditorContextMenuProps) => {
  const selection = useEditorStateStore((state) => state.selection)
  const hasSelection =
    isOpen && Boolean(selection && selection.start.offset !== selection.end.offset)
  const modifierKey = IS_MAC ? 'Cmd' : 'Ctrl'
  const altKey = IS_MAC ? 'Option' : 'Alt'

  if (!isOpen) return null

  const items = buildEditorContextMenuItems({
    hasSelection,
    modifierKey,
    altKey,
    ...handlers,
  })

  return <ContextMenu isOpen={isOpen} position={position} items={items} onClose={onClose} />
}

export default EditorContextMenu
