import { useCallback, useEffect, useReducer, useRef, useState } from 'react'
import {
  ArrowLeft,
  ArrowRight,
  ArrowClockwise as RotateCw,
  GlobeHemisphereWest as Globe,
} from '@phosphor-icons/react'
import { cn } from '@/utils/cn'
import { isTauri } from '@/lib/crowbar-bridge'

export interface WebViewerProps {
  url?: string
  bufferId?: string
  isActive?: boolean
  isVisible?: boolean
  [key: string]: unknown
}

function normalizeUrl(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return 'about:blank'
  if (trimmed.startsWith('about:')) return trimmed
  if (/^https?:\/\//i.test(trimmed)) {
    // Reject bare scheme with no host (e.g. "https://")
    const hasHost = /^https?:\/\/[^/\s]+/i.test(trimmed)
    if (hasHost) return trimmed
  }
  if (/^[^\s/]+\.[^\s/]+/.test(trimmed)) return `https://${trimmed}`
  return `https://www.google.com/search?q=${encodeURIComponent(trimmed)}`
}

function toProxySrc(url: string): string {
  if (url === 'about:blank') return 'about:blank'
  if (!isTauri()) return url
  if (url.startsWith('https://')) return `crowbar-browser://proxy/https/${url.slice(8)}`
  if (url.startsWith('http://')) return `crowbar-browser://proxy/http/${url.slice(7)}`
  return url
}

interface NavState {
  url: string
  canGoBack: boolean
  canGoForward: boolean
}

type NavAction =
  | { type: 'navigate'; url: string; canGoBack: boolean; canGoForward: boolean }

function navReducer(_state: NavState, action: NavAction): NavState {
  switch (action.type) {
    case 'navigate':
      return { url: action.url, canGoBack: action.canGoBack, canGoForward: action.canGoForward }
  }
}

export function WebViewer({
  url: initialUrl = 'about:blank',
  isActive,
}: WebViewerProps) {
  // initialUrl is only read on mount; subsequent navigation is driven by
  // postMessage events from the injected script and user address-bar submits.
  const normalizedInitial = normalizeUrl(initialUrl)
  const iframeRef = useRef<HTMLIFrameElement>(null)

  const [nav, dispatch] = useReducer(navReducer, {
    url: normalizedInitial,
    canGoBack: false,
    canGoForward: false,
  })

  const [inputValue, setInputValue] = useState(normalizedInitial)

  useEffect(() => {
    function handleMessage(e: MessageEvent) {
      if (!e.data || e.data.type !== '__crowbar_browser_nav__') return
      // Only accept nav events from our own iframe (e.source is available cross-origin).
      if (e.source !== iframeRef.current?.contentWindow) return
      const { url, canGoBack, canGoForward } = e.data as {
        url: string
        canGoBack: boolean
        canGoForward: boolean
      }
      dispatch({ type: 'navigate', url, canGoBack, canGoForward })
      setInputValue(url)
    }
    window.addEventListener('message', handleMessage)
    return () => window.removeEventListener('message', handleMessage)
  }, [])

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      const normalized = normalizeUrl(inputValue)
      setInputValue(normalized)
      if (iframeRef.current) {
        iframeRef.current.src = toProxySrc(normalized)
      }
    },
    [inputValue],
  )

  const sendCmd = useCallback((cmd: 'back' | 'forward' | 'reload') => {
    // '*' is safe: back/forward/reload carry no sensitive data, and the iframe
    // origin changes with navigation so it cannot be predicted at call time.
    iframeRef.current?.contentWindow?.postMessage({ type: '__crowbar_cmd__', cmd }, '*')
  }, [])

  return (
    <div className={cn('flex h-full flex-col overflow-hidden', !isActive && 'pointer-events-none')}>
      <form
        onSubmit={handleSubmit}
        className="flex shrink-0 items-center gap-1 border-b border-border bg-card px-2 py-1.5"
      >
        <button
          type="button"
          title="Back"
          disabled={!nav.canGoBack}
          onClick={() => sendCmd('back')}
          className="flex items-center justify-center rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
        >
          <ArrowLeft size={14} />
        </button>
        <button
          type="button"
          title="Forward"
          disabled={!nav.canGoForward}
          onClick={() => sendCmd('forward')}
          className="flex items-center justify-center rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
        >
          <ArrowRight size={14} />
        </button>
        <button
          type="button"
          title="Reload"
          onClick={() => sendCmd('reload')}
          className="flex items-center justify-center rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <RotateCw size={14} />
        </button>

        <div className="flex min-w-0 flex-1 items-center gap-1.5 rounded-md bg-background px-2 py-1 ring-1 ring-border focus-within:ring-primary">
          <Globe size={12} className="shrink-0 text-muted-foreground" />
          <input
            type="text"
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            onFocus={(e) => e.target.select()}
            placeholder="Enter URL or search…"
            className="min-w-0 flex-1 bg-transparent text-xs text-foreground outline-none placeholder:text-muted-foreground"
            spellCheck={false}
            autoCorrect="off"
            autoCapitalize="off"
          />
        </div>
      </form>

      <iframe
        ref={iframeRef}
        src={toProxySrc(normalizedInitial)}
        className="min-h-0 flex-1 w-full border-0 bg-background"
        title="Browser"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-downloads allow-modals"
      />
    </div>
  )
}

export default WebViewer
