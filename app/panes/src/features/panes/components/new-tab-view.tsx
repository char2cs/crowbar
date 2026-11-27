import { FilePlusIcon, GitPullRequest, GlobeIcon, TerminalWindowIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { CrowbarWordmark } from '@/components/ui/crowbar-wordmark'
import { usePaneCanReviewBranch } from '@/features/panes/hooks/use-pane-can-review-branch'
import { usePaneNewTabActions } from '@/features/tabs/hooks/use-pane-new-tab-actions'
import { IS_MOBILE } from '@/utils/platform'

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
  const canReview = usePaneCanReviewBranch(paneId, wsId)
  const v = VARIANTS[IS_MOBILE ? 'phone' : 'desktop']
  return (
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
