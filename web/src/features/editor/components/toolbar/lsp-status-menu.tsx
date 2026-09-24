import { useEffect, useRef, useState } from 'react'
import { Lightning as Zap, LightningSlash as ZapOff } from '@phosphor-icons/react'
import { LspClient, type LspServerStatus } from '@/features/editor/lsp/lsp-client'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { Dropdown } from '@/components/ui/dropdown'
import { toast } from '@/features/window/stores/toast-store'
import { cn } from '@/utils/cn'

const actionButtonClass = cn(buttonVariants({ variant: 'ghost' }), 'rounded text-muted-foreground')

function describe(status: LspServerStatus | null): { title: string; detail?: string } {
  const server = status?.command ? `${status.command}` : 'Language server'
  switch (status?.state) {
    case 'running':
      return { title: `${server} running` }
    case 'stopped':
      return { title: `${server} not running`, detail: 'It starts when a file opens.' }
    case 'notInstalled':
      return {
        title: `${server} not installed`,
        detail: `Install ${server} on your PATH to get completions, hover and diagnostics.`,
      }
    case 'unsupported':
      return { title: 'No language server for this file type' }
    default:
      return { title: 'Language server status unavailable' }
  }
}

/**
 * The language-server status chip for the active file: what the DAEMON says
 * (GET /lsp/status), refreshed when the file changes, when its document
 * finishes opening (that is when a server spawns) and when the menu opens —
 * never polled. Restart and Start act on the daemon and show its answer.
 */
export function LspStatusMenu({ workspaceId, path }: { workspaceId: string; path: string }) {
  const [status, setStatus] = useState<LspServerStatus | null>(null)
  const [isOpen, setIsOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [revision, setRevision] = useState(0)
  const buttonRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    return LspClient.getInstance().onDocumentOpened((openedPath) => {
      if (openedPath === path) setRevision((r) => r + 1)
    })
  }, [path])

  useEffect(() => {
    let cancelled = false
    LspClient.getInstance()
      .status(workspaceId, path)
      .then((next) => {
        if (!cancelled) setStatus(next)
      })
      .catch(() => {
        if (!cancelled) setStatus(null)
      })
    return () => {
      cancelled = true
    }
  }, [workspaceId, path, isOpen, revision])

  const run = async (action: () => Promise<LspServerStatus | null | void>) => {
    setBusy(true)
    try {
      const next = await action()
      if (next) setStatus(next)
      setRevision((r) => r + 1)
    } catch (error) {
      toast.error(
        'Language server action failed',
        error instanceof Error ? error.message : undefined,
      )
    } finally {
      setBusy(false)
    }
  }

  const restart = () => run(() => LspClient.getInstance().restart(workspaceId, path))
  const start = () =>
    run(async () => {
      window.dispatchEvent(new Event('flush-editor-content'))
      const buffer = windowPaneStore
        .getState()
        .buffers.find((b) => isEditorContent(b) && b.path === path && b.workspaceId === workspaceId)
      const content = buffer && isEditorContent(buffer) ? buffer.content : ''
      const languageId = getLanguageIdFromPath(path) ?? 'plaintext'
      await LspClient.getInstance().reopen(path, content, languageId)
    })

  const running = status?.state === 'running'
  const { title, detail } = describe(status)

  return (
    <div className="relative flex items-center self-center">
      <Button
        ref={buttonRef}
        type="button"
        onClick={() => setIsOpen((open) => !open)}
        variant="ghost"
        compact
        className={cn(
          actionButtonClass,
          running ? 'text-green-400' : 'text-muted-foreground opacity-50',
          isOpen && 'bg-muted text-foreground',
        )}
        aria-label="Language server status"
        tooltip={title}
        tooltipSide="bottom"
      >
        <span className="flex size-full items-center justify-center">
          {running ? <Zap /> : <ZapOff />}
        </span>
      </Button>
      <Dropdown
        isOpen={isOpen}
        anchorRef={buttonRef}
        anchorSide="bottom"
        anchorAlign="end"
        onClose={() => setIsOpen(false)}
        className="w-[260px] overflow-hidden rounded-lg p-2"
      >
        <div className="space-y-2 px-1">
          <div className="flex items-center justify-between gap-2">
            <span className="truncate text-foreground ui-text-xs">{title}</span>
            {status?.state === 'running' && (
              <Button
                type="button"
                onClick={() => void restart()}
                disabled={busy}
                variant="default"
                compact
                className="rounded-md px-2 ui-text-xs text-muted-foreground"
              >
                {busy ? 'Restarting…' : 'Restart'}
              </Button>
            )}
            {status?.state === 'stopped' && (
              <Button
                type="button"
                onClick={() => void start()}
                disabled={busy}
                variant="default"
                compact
                className="rounded-md px-2 ui-text-xs text-muted-foreground"
              >
                {busy ? 'Starting…' : 'Start'}
              </Button>
            )}
          </div>
          {detail && <div className="ui-text-xs text-muted-foreground">{detail}</div>}
        </div>
      </Dropdown>
    </div>
  )
}
