import type { KeyboardEvent } from 'react'
import type { SlashCatalogItem } from '@/features/agent/api/agent-chat-reads'
import { matchesPromptStashShortcut } from '@/features/agent/composer/lib/prompt-stash-persistence'
import type { CaretEdges } from '@/features/agent/composer/plate/chat-markdown-editor'

interface ComposerKeyTargets {
  slash: {
    open: boolean
    items: SlashCatalogItem[]
    highlighted?: SlashCatalogItem
    close: () => void
    move: (direction: 1 | -1) => void
  }
  history: {
    recallOlder: (current: string) => string | undefined
    recallNewer: () => string | undefined
  }
  /** The composer's own stop button is showing. */
  canStop: boolean
  stop: () => void
  stashOrRestore: (markdown: string) => void
  selectSlashItem: (item: SlashCatalogItem) => void
  send: (markdown: string) => void
  recall: (text: string) => void
}

/** One key in the composer, routed to the picker, the queue or history recall. */
export function handleComposerKey(
  targets: ComposerKeyTargets,
  event: KeyboardEvent<HTMLDivElement>,
  readMarkdown: () => string,
  caret: CaretEdges,
) {
  const { slash } = targets
  if (matchesPromptStashShortcut(event)) {
    event.preventDefault()
    event.stopPropagation()
    targets.stashOrRestore(readMarkdown())
    return
  }
  if (event.key === 'Escape' && (slash.open || targets.canStop)) {
    event.preventDefault()
    if (slash.open) slash.close()
    else targets.stop()
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
  if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
    event.preventDefault()
    // Enter completes when there is a completion, else sends: the catalogue is
    // incomplete by declaration, so a built-in like /compact matches nothing.
    if (slash.highlighted) targets.selectSlashItem(slash.highlighted)
    else targets.send(readMarkdown())
    return
  }
  // History recall only at an edge the caret could not otherwise move past.
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
