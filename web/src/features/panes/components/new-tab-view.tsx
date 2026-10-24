import { FilePlusIcon, GitPullRequest, GlobeIcon, TerminalWindowIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { CrowbarWordmark } from '@/components/ui/crowbar-wordmark'
import { useChatIsThread } from '@/features/panes/hooks/use-chat-is-thread'
import { usePaneById } from '@/features/workspace/stores/hooks/use-pane-store'
import { usePaneNewTabActions } from '@/features/tabs/hooks/use-pane-new-tab-actions'

const ACTION_CLASS = 'justify-start text-muted-foreground'

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
  // A thread owns no branch to review (same gate as the header's shortcut).
  const isThread = useChatIsThread(usePaneById(paneId)?.chatId ?? null)
  return (
    // Focusable so a click on the blank area puts focus in the editor area,
    // where ⌘N opens a New Tab (use-pane-keyboard.ts).
    <div
      tabIndex={-1}
      className="relative flex h-full w-full flex-col items-center justify-center gap-6 overflow-hidden outline-none"
    >
      <CrowbarWordmark
        aria-hidden="true"
        className="pointer-events-none h-auto w-[clamp(96px,14cqmin,148px)] text-muted-foreground"
      />
      {wsId && (
        <div className="flex flex-col items-stretch gap-0.5">
          <Button variant="ghost" size="sm" className={ACTION_CLASS} onClick={onNewTerminal}>
            <TerminalWindowIcon />
            New Terminal
          </Button>
          <Button variant="ghost" size="sm" className={ACTION_CLASS} onClick={onNewBrowser}>
            <GlobeIcon />
            New Browser
          </Button>
          <Button variant="ghost" size="sm" className={ACTION_CLASS} onClick={onNewFile}>
            <FilePlusIcon />
            New File
          </Button>
          {!isThread && (
            <Button variant="ghost" size="sm" className={ACTION_CLASS} onClick={onReviewBranch}>
              <GitPullRequest />
              Review Branch
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
