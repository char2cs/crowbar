import { Asterisk, CaretDown, CaretUp, MagnifyingGlass, TextAa, X } from '@phosphor-icons/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { actionErrorMessage } from '@/features/hosts/lib/host-failure-notice'
import { searchReviewDiff } from '@/features/git/api/review-window-api'
import type { SearchDiffResult, SearchHit } from '@/features/git/api/review-window-api'
import { cn } from '@/utils/cn'

export const SEARCH_DEBOUNCE_MS = 200

const EMPTY_RESULT: SearchDiffResult = { hits: [], truncated: false }

export interface ReviewSearchBarProps {
  wsId: string
  commit?: string
  onSelectHit: (hit: SearchHit) => void
  onClose?: () => void
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
  const [activeIndex, setActiveIndex] = useState(-1)

  const generation = useRef(0)

  useEffect(() => {
    const gen = ++generation.current

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
          setResult(EMPTY_RESULT)
          setError(actionErrorMessage(err, 'Search failed'))
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
