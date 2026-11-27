import type { KeyboardEvent } from 'react'
import type { PickerItem } from '@/features/agent/composer/composer-slash-picker'
import { matchesPromptStashShortcut } from '@/features/agent/composer/lib/prompt-stash-persistence'
import type {
  CaretEdges,
  ComposerShape,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { isSendKey } from '@/features/agent/chat/lib/send-key'

interface ComposerKeyTargets {
  slash: {
    open: boolean
    items: PickerItem[]
    highlighted?: PickerItem
    close: () => void
    move: (direction: 1 | -1) => void
  }
  history: {
    recallOlder: (current: string) => string | undefined
    recallNewer: () => string | undefined
    searchOlder: (typed: string) => string | undefined
  }
  canStop: boolean
  stop: () => void
  retract: () => boolean
  stashOrRestore: (markdown: string) => void
  selectSlashItem: (item: PickerItem) => void
  send: (markdown: string) => void
  enterSends: boolean
  recall: (text: string) => void
}

export function handleComposerKey(
  targets: ComposerKeyTargets,
  event: KeyboardEvent<HTMLDivElement>,
  readMarkdown: () => string,
  caret: CaretEdges,
  shape: ComposerShape,
) {
  const { slash } = targets
  if (matchesPromptStashShortcut(event)) {
    event.preventDefault()
    event.stopPropagation()
    targets.stashOrRestore(readMarkdown())
    return
  }
  if (event.key === 'Escape') {
    if (slash.open) slash.close()
    else if (!shape.empty || !targets.retract()) {
      if (!targets.canStop) return
      targets.stop()
    }
    event.preventDefault()
    return
  }
  const arrow = event.key === 'ArrowDown' || event.key === 'ArrowUp'
  if (slash.open && slash.items.length > 0 && arrow) {
    event.preventDefault()
    slash.move(event.key === 'ArrowDown' ? 1 : -1)
    return
  }
  if (event.key === 'Tab' && !event.shiftKey && slash.highlighted) {
    event.preventDefault()
    targets.selectSlashItem(slash.highlighted)
    return
  }
  const plainEnter = !event.metaKey && !event.ctrlKey
  if (isSendKey(event) && (targets.enterSends || !plainEnter || slash.highlighted)) {
    event.preventDefault()
    const draft = readMarkdown()
    const { highlighted } = slash
    const typedInFull =
      highlighted?.insertText.startsWith('/') && draft.trim() === highlighted.insertText.trim()
    if (highlighted && !typedInFull) targets.selectSlashItem(highlighted)
    else targets.send(draft)
    return
  }
  if (event.key === 'r' && event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey) {
    event.preventDefault()
    const found = targets.history.searchOlder(readMarkdown())
    if (found !== undefined) targets.recall(found)
    return
  }
  const plain = !event.shiftKey && !event.metaKey && !event.ctrlKey && !event.altKey
  if (slash.open || !plain) return
  const recalled =
    event.key === 'ArrowUp' && caret.atStart
      ? targets.history.recallOlder(readMarkdown())
      : event.key === 'ArrowDown' && caret.atEnd
        ? targets.history.recallNewer()
        : undefined
  if (recalled === undefined) return
  event.preventDefault()
  targets.recall(recalled)
}
