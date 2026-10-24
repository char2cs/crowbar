import { useEffect, useRef, useState, type FormEvent } from 'react'
import { ArrowClockwiseIcon, ArrowLeftIcon, ArrowRightIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { bestEffort } from '@/lib/best-effort'
import { useBufferActions, useBufferById } from '@/features/workspace/stores/hooks/use-buffer-store'
import { useBrowserOccluded } from '@/features/browser/hooks/use-browser-occluded'
import { useBrowserWebview } from '@/features/browser/hooks/use-browser-webview'
import { browserApi } from '@/lib/browser-bridge'
import { useBrowserAgentStore } from '@/features/browser/lib/agent-activity-store'
import { useSettingsStore } from '@/features/settings/store'
import { searchTemplateFor } from '@/features/browser/lib/search-engines'
import { normalizeBrowserUrl } from '@/features/browser/lib/normalize-url'

interface BrowserPaneProps {
  paneId: string
  bufferId: string
  /** This tab is the one on screen: the active tab of a shown, unhidden pane. */
  visible: boolean
}

export function BrowserPane({ paneId, bufferId, visible }: BrowserPaneProps) {
  const buffer = useBufferById(bufferId)
  const { setBrowserLocation } = useBufferActions()
  const occluded = useBrowserOccluded(paneId)
  const placeholderRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const [draft, setDraft] = useState<string | null>(null)
  const searchEngine = useSettingsStore((s) => s.settings.browserSearchEngine)
  const customSearchUrl = useSettingsStore((s) => s.settings.browserCustomSearchUrl)

  const agentActive = useBrowserAgentStore((s) => (s.active[bufferId] ?? 0) > 0)
  const url = buffer?.type === 'browser' ? buffer.url : ''
  const hasUrl = url !== ''

  useBrowserWebview({
    bufferId,
    hasUrl,
    visible: visible && !occluded,
    placeholderRef,
  })

  useEffect(() => {
    if (!hasUrl && visible) inputRef.current?.focus()
  }, [hasUrl, visible])

  const submit = (event: FormEvent) => {
    event.preventDefault()
    const next = normalizeBrowserUrl(draft ?? url, searchTemplateFor(searchEngine, customSearchUrl))
    if (!next) return
    if (hasUrl) bestEffort(browserApi.navigate(bufferId, next), 'navigate browser webview')
    setBrowserLocation(bufferId, { url: next })
    setDraft(null)
  }

  return (
    <div className="flex h-full flex-col bg-background">
      <form
        onSubmit={submit}
        className="flex shrink-0 items-center gap-1 border-b border-border px-2 py-1.5"
      >
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Back"
          disabled={!hasUrl}
          onClick={() => bestEffort(browserApi.back(bufferId), 'browser back')}
        >
          <ArrowLeftIcon />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Forward"
          disabled={!hasUrl}
          onClick={() => bestEffort(browserApi.forward(bufferId), 'browser forward')}
        >
          <ArrowRightIcon />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Reload"
          disabled={!hasUrl}
          onClick={() => bestEffort(browserApi.reload(bufferId), 'browser reload')}
        >
          <ArrowClockwiseIcon />
        </Button>
        {agentActive && <Badge variant="info">Agent</Badge>}
        <Input
          ref={inputRef}
          aria-label="Address"
          size="sm"
          placeholder="Search or enter address"
          value={draft ?? url}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={() => setDraft(null)}
        />
      </form>
      <div ref={placeholderRef} className="min-h-0 flex-1" />
    </div>
  )
}
