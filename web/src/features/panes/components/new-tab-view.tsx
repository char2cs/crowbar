import { lazy, Suspense } from 'react'
import { CrowbarWordmark } from '@/components/ui/crowbar-wordmark'

// Lazy so the BAKED POINT CLOUD stays off the startup path. ascii-crowbar pulls
// in ascii-crowbar-cloud.ts, a single 180,000-character base64 literal, and a
// static import from here would land it in the entry shell chunk — paid by
// every cold start for a decoration only ever drawn on an empty pane. Same
// treatment editor-pane.tsx gives Plate.
//
// The fallback is `null` and that is correct, not lazy shorthand: this is a
// pointer-events-none, aria-hidden backdrop positioned `absolute inset-0`, so
// it occupies no layout and its late arrival shifts nothing on the surface.
const AsciiCrowbar = lazy(() => import('@/features/panes/components/ascii-crowbar'))

/**
 * The surface a pane shows when its editor view holds no active buffer. Under
 * the New Tab rules a pane always holds at least a chat or a buffer, so this
 * should be unreachable — kept, pointed at the same component every stranded
 * pane renders, so a bug that ever does leave one bare shows something
 * instead of a blank rectangle with no way out.
 *
 * Deliberately inert past the ambient backdrop: the wordmark, centred, over a
 * slowly tumbling ASCII-art crowbar, and nothing else. This used to offer New
 * Terminal / New File / Review the Branch / a chat history list — real
 * actions, each of which silently minted a brand-new, redundant chat for a
 * pane that merely hadn't been told its workspace's REAL owning chat yet
 * (`ensurePaneChatThenOpen` in pane-command-actions.ts). A pane with nothing
 * open must never be a screen a user can act from, so this offers nothing to
 * click at all — not the terminal/file/review actions, not the chat history,
 * not a "New Chat" quick-start. The tumbling backdrop is decoration only
 * (`pointer-events-none`, `aria-hidden`), so it does not reopen that hole.
 */
export function NewTabView({ paneId }: { paneId: string }) {
  return (
    <div className="relative flex h-full w-full items-center justify-center overflow-hidden">
      {/* Seeded by paneId so two empty panes on screen at once don't tumble in
          lockstep. */}
      <Suspense fallback={null}>
        <AsciiCrowbar seed={paneId} />
      </Suspense>
      <CrowbarWordmark
        aria-hidden="true"
        className="pointer-events-none relative h-auto w-[clamp(96px,14cqmin,148px)] text-muted-foreground"
      />
    </div>
  )
}
