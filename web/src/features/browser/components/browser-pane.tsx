import { useEffect, useRef, useState, type FormEvent } from 'react'
import {
  ArrowClockwiseIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  CrosshairIcon,
} from '@phosphor-icons/react'
import { useStore } from 'zustand'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { bestEffort } from '@/lib/best-effort'
import { useBufferActions, useBufferById } from '@/features/workspace/stores/hooks/use-buffer-store'
import { useBrowserOccluded } from '@/features/browser/hooks/use-browser-occluded'
import { usePointerPick } from '@/features/browser/hooks/use-pointer-pick'
import { useBrowserWebview } from '@/features/browser/hooks/use-browser-webview'
import { browserApi } from '@/lib/browser-bridge'
import { useBrowserAgentStore } from '@/features/browser/lib/agent-activity-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { BROWSER_TOGGLE_PICK } from '@/features/keymaps/registry'
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

  const { picking, toggle: togglePick } = usePointerPick(bufferId, visible && !occluded)
  const focused = useStore(windowPaneStore, (s) => s.activePaneId === paneId)
  const pickChord = useEffectiveChordMap()[BROWSER_TOGGLE_PICK]
  useEffect(() => {
    if (!hasUrl || !visible || !focused || !pickChord) return
    const onKey = (e: KeyboardEvent) => {
      if (e.repeat || !eventMatchesChord(e, pickChord)) return
      e.preventDefault()
      togglePick()
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [hasUrl, visible, focused, pickChord, togglePick])

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
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Pick element"
          aria-pressed={picking}
          tooltip="Pick element (copy pointer)"
          active={picking}
          disabled={!hasUrl}
          onClick={togglePick}
        >
          <CrosshairIcon />
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
