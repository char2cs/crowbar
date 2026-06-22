import { CaretUp, CaretDown, TextAa, X } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/utils/cn'
import type { DiffSearchState } from './use-diff-search'

interface DiffSearchBarProps {
  search: DiffSearchState
  onClose: () => void
}

/** Compact find bar for the multi-file diff: query, match count, next/prev, case toggle. */
export function DiffSearchBar({ search, onClose }: DiffSearchBarProps) {
  const { query, setQuery, total, currentIndex, limited, caseSensitive, toggleCaseSensitive, next, prev } =
    search
  const trimmed = query.trim()
  const hasNoResults = trimmed !== '' && total === 0
  const countLabel =
    trimmed === ''
      ? ''
      : total === 0
        ? 'No results'
        : `${currentIndex + 1}/${total}${limited ? '+' : ''}`

  return (
    <div
      role="search"
      className="flex shrink-0 items-center gap-1 border-border border-b bg-background px-2 py-1.5"
    >
      <Input
        size="sm"
        autoFocus
        aria-label="Search diff"
        value={query}
        placeholder="Search diff…"
        onChange={(event) => setQuery(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.preventDefault()
            if (event.shiftKey) prev()
            else next()
          } else if (event.key === 'Escape') {
            event.preventDefault()
            onClose()
          }
        }}
        className={cn('max-w-xs flex-1', hasNoResults && 'text-destructive')}
      />
      <span
        className={cn(
          'min-w-[3.25rem] text-right ui-text-xs tabular-nums',
          hasNoResults ? 'text-destructive' : 'text-muted-foreground',
        )}
      >
        {countLabel}
      </span>
      <Button
        size="xs"
        variant="ghost"
        disabled={total === 0}
        onClick={prev}
        aria-label="Previous match"
        tooltip="Previous match (Shift+Enter)"
        tooltipSide="bottom"
      >
        <CaretUp className="size-3.5" />
      </Button>
      <Button
        size="xs"
        variant="ghost"
        disabled={total === 0}
        onClick={next}
        aria-label="Next match"
        tooltip="Next match (Enter)"
        tooltipSide="bottom"
      >
        <CaretDown className="size-3.5" />
      </Button>
      <Button
        size="xs"
        variant={caseSensitive ? 'default' : 'ghost'}
        onClick={toggleCaseSensitive}
        aria-label="Match case"
        tooltip="Match case"
        tooltipSide="bottom"
      >
        <TextAa className="size-3.5" />
      </Button>
      <Button
        size="xs"
        variant="ghost"
        onClick={onClose}
        aria-label="Close search"
        tooltip="Close (Esc)"
        tooltipSide="bottom"
      >
        <X className="size-3.5" />
      </Button>
    </div>
  )
}
