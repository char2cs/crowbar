import { CrowbarWordmark } from '@/components/ui/crowbar-wordmark'

/**
 * The surface a pane shows when its editor view holds no active buffer. Under
 * the New Tab rules a pane always holds at least a chat or a buffer, so this
 * should be unreachable — kept, pointed at the same component every stranded
 * pane renders, so a bug that ever does leave one bare shows something
 * instead of a blank rectangle with no way out.
 *
 * Deliberately inert: the wordmark, centred, and nothing else. This used to
 * offer New Terminal / New File / Review the Branch / a chat history list —
 * real actions, each of which silently minted a brand-new, redundant chat
 * for a pane that merely hadn't been told its workspace's REAL owning chat
 * yet (`ensurePaneChatThenOpen` in pane-command-actions.ts). A pane with
 * nothing open must never be a screen a user can act from, so this offers
 * nothing to click at all — not the terminal/file/review actions, not the
 * chat history, not a "New Chat" quick-start.
 */
export function NewTabView({ paneId: _paneId }: { paneId: string }) {
  return (
    <div className="flex h-full w-full items-center justify-center overflow-hidden">
      <CrowbarWordmark
        aria-hidden="true"
        className="pointer-events-none h-auto w-[clamp(96px,14cqmin,148px)] text-muted-foreground"
      />
    </div>
  )
}
