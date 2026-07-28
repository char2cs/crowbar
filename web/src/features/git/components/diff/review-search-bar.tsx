import { Asterisk, CaretDown, CaretUp, MagnifyingGlass, TextAa, X } from '@phosphor-icons/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { searchReviewDiff } from '@/features/git/api/review-window-api'
import type { SearchDiffResult, SearchHit } from '@/features/git/api/review-window-api'
import { cn } from '@/utils/cn'

// Find-in-diff for the windowed review surface.
//
// The client only ever holds a WINDOW of the patch, so there is nothing left to
// search locally — every query goes to the daemon, which streams the diff and
// matches a line at a time. That moves two costs onto this component:
//
//   1. Each keystroke would otherwise start a fresh whole-diff scan. The
//      endpoint reads 46 MB in ~700ms when nothing matches, so an undebounced
//      find box queues one such scan per character. Hence the 200ms debounce.
//   2. Those scans finish out of order. A slow early query landing after a fast
//      later one is the classic find-box race, and it shows the reader hits for
//      a query they no longer have typed. Hence the generation guard.
//
// The component owns only its own query and results. Where a hit LANDS — which
// file to materialise, which line to scroll to — belongs to the review surface,
// which receives the hit through onSelectHit.

/** Delay between the last keystroke and the request it triggers.
 *
 *  Not a taste knob: the /review/search endpoint scans the entire branch diff
 *  when a query matches nothing, which measured ~700ms over 46 MB. Firing per
 *  keystroke queues one of those per character. 200ms is long enough that a
 *  typed word costs one scan and short enough to still feel like find-as-you-type. */
export const SEARCH_DEBOUNCE_MS = 200

const EMPTY_RESULT: SearchDiffResult = { hits: [], truncated: false }

export interface ReviewSearchBarProps {
  wsId: string
  /** Scopes the search to ONE commit instead of the workspace's branch. */
  commit?: string
  /** Called with the hit the reader moved to — by clicking it, or via next/prev.
   *  The surface materialises the hit's file and scrolls to its line. */
  onSelectHit: (hit: SearchHit) => void
  /** Rendered as a close affordance and bound to Escape when supplied. */
  onClose?: () => void
  /** Server-side cap on hits. Omitted takes the endpoint's default of 200; the
   *  endpoint clamps anything above its hard maximum of 1000. */
  limit?: number
  className?: string
}

export function ReviewSearchBar({
  wsId,
  commit,
  onSelectHit,
  onClose,
  limit,
  className,
}: ReviewSearchBarProps) {
  const [query, setQuery] = useState('')
  const [useRegex, setUseRegex] = useState(false)
  const [caseSensitive, setCaseSensitive] = useState(false)
  const [result, setResult] = useState<SearchDiffResult>(EMPTY_RESULT)
  const [error, setError] = useState<string | null>(null)
  const [searching, setSearching] = useState(false)
  /** -1 = results exist but the reader has not moved into them yet. New results
   *  never auto-jump: scrolling the diff on every keystroke of a find-as-you-type
   *  box is motion sickness, so the first next/prev is what commits to a hit. */
  const [activeIndex, setActiveIndex] = useState(-1)

  // Monotonic request id. Every run of the effect below — a new query, a toggled
  // option, an unmount — bumps it, which is what makes an older in-flight search
  // stale: its answer is compared against the id current when IT resolves, not
  // when it started, so a superseded response is dropped rather than rendered.
  const generation = useRef(0)

  useEffect(() => {
    const gen = ++generation.current

    // An empty box is the state this opens in. The daemon answers it with an
    // empty result rather than an error, but there is nothing to ask about, so
    // it costs no request at all.
    if (query === '') {
      setResult(EMPTY_RESULT)
      setError(null)
      setSearching(false)
      setActiveIndex(-1)
      return
    }

    const timer = setTimeout(() => {
      setSearching(true)
      searchReviewDiff({ wsId, commit }, query, { regex: useRegex, caseSensitive, limit })
        .then((next) => {
          if (generation.current !== gen) return
          setResult(next)
          setError(null)
          setSearching(false)
          setActiveIndex(-1)
        })
        .catch((err: unknown) => {
          if (generation.current !== gen) return
          // An unparseable regex is a 400 the usecase raises from regexp.Compile
          // before it spawns anything — an ordinary answer to a half-typed
          // pattern, so it belongs inline in the bar, not in an error boundary.
          setResult(EMPTY_RESULT)
          setError(err instanceof Error ? err.message : 'Search failed')
          setSearching(false)
          setActiveIndex(-1)
        })
    }, SEARCH_DEBOUNCE_MS)

    return () => clearTimeout(timer)
  }, [wsId, commit, query, useRegex, caseSensitive, limit])

  const hits = result.hits

  const selectAt = useCallback(
    (index: number) => {
      setActiveIndex(index)
      onSelectHit(hits[index])
    },
    [hits, onSelectHit],
  )

  const move = useCallback(
    (delta: 1 | -1) => {
      if (hits.length === 0) return
      const next =
        activeIndex < 0
          ? delta === 1
            ? 0
            : hits.length - 1
          : (activeIndex + delta + hits.length) % hits.length
      selectAt(next)
    },
    [activeIndex, hits.length, selectAt],
  )

  const positionLabel = activeIndex >= 0 ? `${activeIndex + 1}/${hits.length}` : ''

  return (
    // <search> is the native landmark for this (Baseline 2023) and carries the
    // "search" role for free.
    <search
      className={cn(
        'flex shrink-0 flex-col gap-1 border-border border-b bg-background px-2 py-1.5',
        className,
      )}
    >
      <div className="flex items-center gap-1">
        <Input
          size="sm"
          type="search"
          autoFocus
          name="review-diff-search"
          aria-label="Find in diff"
          placeholder="Find in diff…"
          leftIcon={MagnifyingGlass}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              move(event.shiftKey ? -1 : 1)
            } else if (event.key === 'Escape' && onClose) {
              event.preventDefault()
              onClose()
            }
          }}
          className="min-w-0 flex-1"
        />
        <span className="min-w-[3.25rem] text-right text-muted-foreground ui-text-xs tabular-nums">
          {positionLabel}
        </span>
        <Button
          size="xs"
          variant="ghost"
          disabled={hits.length === 0}
          onClick={() => move(-1)}
          aria-label="Previous match"
          tooltip="Previous match (Shift+Enter)"
          tooltipSide="bottom"
        >
          <CaretUp />
        </Button>
        <Button
          size="xs"
          variant="ghost"
          disabled={hits.length === 0}
          onClick={() => move(1)}
          aria-label="Next match"
          tooltip="Next match (Enter)"
          tooltipSide="bottom"
        >
          <CaretDown />
        </Button>
        <Button
          size="xs"
          variant={caseSensitive ? 'secondary' : 'ghost'}
          aria-pressed={caseSensitive}
          onClick={() => setCaseSensitive((on) => !on)}
          aria-label="Match case"
          tooltip="Match case"
          tooltipSide="bottom"
        >
          <TextAa />
        </Button>
        <Button
          size="xs"
          variant={useRegex ? 'secondary' : 'ghost'}
          aria-pressed={useRegex}
          onClick={() => setUseRegex((on) => !on)}
          aria-label="Use regular expression"
          tooltip="Use regular expression"
          tooltipSide="bottom"
        >
          <Asterisk />
        </Button>
        {onClose && (
          <Button
            size="xs"
            variant="ghost"
            onClick={onClose}
            aria-label="Close search"
            tooltip="Close (Esc)"
            tooltipSide="bottom"
          >
            <X />
          </Button>
        )}
      </div>

      <SearchStatus
        query={query}
        searching={searching}
        error={error}
        hitCount={hits.length}
        truncated={result.truncated}
      />

      {hits.length > 0 && (
        <ul className="max-h-64 overflow-y-auto">
          {/* Keyed on path:side:line, which identifies a hit on its own — the
              scanner records at most ONE hit per line (diff/search.go `record`
              returns early on no match, then appends once). No index tiebreaker
              is needed, so the key stays stable when a new query replaces the
              list. */}
          {hits.map((h, index) => (
            <li key={`${h.path}:${h.side}:${h.lineNumber}`}>
              <button
                type="button"
                aria-current={index === activeIndex ? true : undefined}
                onClick={() => selectAt(index)}
                className={cn(
                  'flex w-full items-baseline gap-2 rounded px-1 py-0.5 text-left hover:bg-accent',
                  index === activeIndex && 'bg-accent',
                )}
              >
                <span className="max-w-[40%] shrink-0 truncate text-foreground ui-text-xs">
                  {h.path}
                </span>
                <span className="shrink-0 text-muted-foreground ui-text-xs tabular-nums">
                  {h.lineNumber}
                </span>
                <span className="truncate font-mono text-muted-foreground ui-text-xs">
                  {h.preview}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </search>
  )
}

/** The one line under the box that says what the results actually are.
 *
 *  Truncation is stated rather than implied: the endpoint caps at `limit`
 *  (default 200, hard maximum 1000) and reports having done so, and a capped
 *  list rendered as if it were complete is a claim the reader cannot check. */
function SearchStatus({
  query,
  searching,
  error,
  hitCount,
  truncated,
}: {
  query: string
  searching: boolean
  error: string | null
  hitCount: number
  truncated: boolean
}) {
  if (error) {
    return (
      <p role="alert" className="text-destructive ui-text-xs">
        {error}
      </p>
    )
  }
  if (query === '') return null
  const text = searching
    ? 'Searching…'
    : truncated
      ? `Showing the first ${hitCount} matches`
      : hitCount === 0
        ? 'No matches'
        : `${hitCount} ${hitCount === 1 ? 'match' : 'matches'}`
  return <p className="text-muted-foreground ui-text-xs">{text}</p>
}
