import { useState, useRef, useCallback } from 'react'
import {
  ArrowClockwise as RotateCw,
  GlobeHemisphereWest as Globe,
} from '@phosphor-icons/react'
import { cn } from '@/utils/cn'

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
  // Looks like a domain (contains a dot, no spaces)
  if (/^[^\s/]+\.[^\s/]+/.test(trimmed)) return `https://${trimmed}`
  // Treat as a search query
  return `https://www.google.com/search?q=${encodeURIComponent(trimmed)}`
}

export function WebViewer({ url: initialUrl = 'about:blank', isActive }: WebViewerProps) {
  const [src, setSrc] = useState(() => normalizeUrl(initialUrl))
  const [inputValue, setInputValue] = useState(src)
  const [key, setKey] = useState(0) // bump to force iframe reload
  const iframeRef = useRef<HTMLIFrameElement>(null)

  const navigate = useCallback((raw: string) => {
    const normalized = normalizeUrl(raw)
    setSrc(normalized)
    setInputValue(normalized)
  }, [])

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      navigate(inputValue)
    },
    [inputValue, navigate],
  )

  const handleReload = useCallback(() => {
    setKey(k => k + 1)
  }, [])

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
      {src === 'about:blank' || src === 'https://' ? (
        <div className="flex flex-1 items-center justify-center text-muted-foreground ui-text-sm">
          Enter a URL above to browse
        </div>
      ) : (
        <iframe
          key={key}
          ref={iframeRef}
          src={src}
          className="min-h-0 flex-1 border-none"
          sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-downloads"
          title="Web Viewer"
        />
      )}
    </div>
  )
}

export default WebViewer
