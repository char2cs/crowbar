import { useState, useRef, useCallback, useEffect } from 'react'
import {
  ArrowClockwise as RotateCw,
  GlobeHemisphereWest as Globe,
} from '@phosphor-icons/react'
import { cn } from '@/utils/cn'
import { browserPaneNavigate, browserPaneReload } from '@/lib/crowbar-bridge'
import { useBrowserPaneAnchor } from '@/features/web-viewer/hooks/use-browser-pane-anchor'
import { useWebViewerNavigationStore } from '@/features/web-viewer/stores/web-viewer-navigation-store'

export interface WebViewerProps {
  url?: string
  bufferId?: string
  profileKey?: string
  history?: string[]
  historyIndex?: number
  isActive?: boolean
  isVisible?: boolean
  [key: string]: unknown
}

function normalizeUrl(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return 'about:blank'
  if (trimmed.startsWith('about:')) return trimmed
  if (/^https?:\/\//i.test(trimmed)) return trimmed
  if (/^[^\s/]+\.[^\s/]+/.test(trimmed)) return `https://${trimmed}`
  return `https://www.google.com/search?q=${encodeURIComponent(trimmed)}`
}

export function WebViewer({
  url: initialUrl = 'about:blank',
  bufferId = '',
  isActive,
  isVisible = true,
}: WebViewerProps) {
  const normalizedInitialUrl = normalizeUrl(initialUrl)
  const anchorRef = useRef<HTMLDivElement>(null)
  // Pass the initial URL to the hook so it's sent with the first browserPaneSync
  // call. This avoids a race where browserPaneNavigate fires before the webview exists.
  const { isTauri } = useBrowserPaneAnchor({
    bufferId,
    isVisible,
    anchorRef,
    initialUrl: normalizedInitialUrl !== 'about:blank' ? normalizedInitialUrl : undefined,
  })

  const navEntry = useWebViewerNavigationStore(state =>
    bufferId ? state.navigationByBufferId[bufferId] : undefined,
  )

  // Register this buffer in the nav store; initial URL drives the address bar
  useEffect(() => {
    if (!bufferId) return
    const { registerBuffer, removeBuffer } = useWebViewerNavigationStore.getState()
    registerBuffer(bufferId, normalizedInitialUrl)
    return () => {
      useWebViewerNavigationStore.getState().removeBuffer(bufferId)
    }
  }, [bufferId]) // eslint-disable-line react-hooks/exhaustive-deps

  // Address bar follows the nav store url; falls back to normalized initial url
  const [inputValue, setInputValue] = useState(() => normalizeUrl(initialUrl))
  useEffect(() => {
    if (navEntry?.url) setInputValue(navEntry.url)
  }, [navEntry?.url])

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      const url = normalizeUrl(inputValue)
      setInputValue(url)
      if (bufferId) void browserPaneNavigate(bufferId, url)
    },
    [bufferId, inputValue],
  )

  const handleReload = useCallback(() => {
    if (bufferId) void browserPaneReload(bufferId)
  }, [bufferId])

  return (
    <div className={cn('flex h-full flex-col overflow-hidden', !isActive && 'pointer-events-none')}>
      {/* Navigation bar */}
      <form
        onSubmit={handleSubmit}
        className="flex shrink-0 items-center gap-1 border-b border-border bg-card px-2 py-1.5"
      >
        <button
          type="button"
          title="Reload"
          onClick={handleReload}
          className="flex items-center justify-center rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <RotateCw size={14} />
        </button>

        <div className="flex min-w-0 flex-1 items-center gap-1.5 rounded-md bg-background px-2 py-1 ring-1 ring-border focus-within:ring-primary">
          <Globe size={12} className="shrink-0 text-muted-foreground" />
          <input
            type="text"
            value={inputValue}
            onChange={e => setInputValue(e.target.value)}
            onFocus={e => e.target.select()}
            placeholder="Enter URL or search…"
            className="min-w-0 flex-1 bg-transparent text-xs text-foreground outline-none placeholder:text-muted-foreground"
            spellCheck={false}
            autoCorrect="off"
            autoCapitalize="off"
          />
        </div>
      </form>

      {/* Content */}
      {!isTauri ? (
        <div className="flex flex-1 items-center justify-center text-muted-foreground ui-text-sm">
          This feature requires the desktop app
        </div>
      ) : (
        <div
          ref={anchorRef}
          data-browser-anchor
          className="min-h-0 flex-1"
        />
      )}
    </div>
  )
}

export default WebViewer
