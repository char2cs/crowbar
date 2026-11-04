import { FilePlusIcon, GitPullRequest, GlobeIcon, TerminalWindowIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { CrowbarWordmark } from '@/components/ui/crowbar-wordmark'
import { usePaneCanReviewBranch } from '@/features/panes/hooks/use-pane-can-review-branch'
import { usePaneNewTabActions } from '@/features/tabs/hooks/use-pane-new-tab-actions'
import { IS_MOBILE } from '@/utils/platform'

// The phone fills a whole screen with this view: a 1.6x logo over touch-sized (44pt) rows.
const VARIANTS = {
  desktop: {
    action: 'justify-start text-muted-foreground',
    size: 'sm',
    logo: 'pointer-events-none h-auto w-[clamp(96px,14cqmin,148px)] text-muted-foreground',
    logoWidth: undefined,
  },
  phone: {
    action: 'min-h-11 justify-start text-muted-foreground',
    size: 'xl',
    logo: 'pointer-events-none h-auto text-muted-foreground',
    logoWidth: 154,
  },
} as const

/**
 * What an editor area shows while it holds no tab, and what a New Tab page
 * (`replaceTabId`, which a choice replaces) shows: the Crowbar logo (the one
 * onboarding uses) and, once the pane's workspace is resolved, its actions.
 *
 * The actions need `wsId` because `ensurePaneChatThenOpen` mints a redundant
 * chat for a pane that has not been told its workspace's owning chat yet;
 * until then only the logo is drawn.
 */
export function NewTabView({
  paneId,
  wsId,
  replaceTabId,
}: {
  paneId: string
  wsId: string | null
  replaceTabId?: string
}) {
  const { onNewTerminal, onNewBrowser, onNewFile, onReviewBranch } = usePaneNewTabActions(
    wsId,
    paneId,
    replaceTabId,
  )
  const canReview = usePaneCanReviewBranch(paneId)
  const v = VARIANTS[IS_MOBILE ? 'phone' : 'desktop']
  return (
    // Focusable so a click on the blank area puts focus in the editor area,
    // where ⌘N opens a New Tab (use-pane-keyboard.ts).
    <div
      tabIndex={-1}
      data-variant={IS_MOBILE ? 'phone' : undefined}
      className="relative flex h-full w-full flex-col items-center justify-center gap-6 overflow-hidden outline-none data-[variant=phone]:gap-8"
    >
      <CrowbarWordmark aria-hidden="true" width={v.logoWidth} className={v.logo} />
      {wsId && (
        <div className="flex flex-col items-stretch gap-0.5 in-data-[variant=phone]:gap-1">
          <Button variant="ghost" size={v.size} className={v.action} onClick={onNewTerminal}>
            <TerminalWindowIcon />
            New Terminal
          </Button>
          {/* The phone has no browser pane and no file editor yet. */}
          {!IS_MOBILE && (
            <>
              <Button variant="ghost" size={v.size} className={v.action} onClick={onNewBrowser}>
                <GlobeIcon />
                New Browser
              </Button>
              <Button variant="ghost" size={v.size} className={v.action} onClick={onNewFile}>
                <FilePlusIcon />
                New File
              </Button>
            </>
          )}
          {canReview && (
            <Button variant="ghost" size={v.size} className={v.action} onClick={onReviewBranch}>
              <GitPullRequest />
              Review Branch
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
