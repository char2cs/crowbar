import { cva } from 'class-variance-authority'
import {
  CaretDown as ChevronDown,
  CaretRight as ChevronRight,
  CaretUp as ChevronUp,
  ArrowsLeftRight as Replace,
  MagnifyingGlass as Search,
  X,
} from '@phosphor-icons/react'
import type { ReactNode, RefObject } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/utils/cn'

export interface SearchToggleOption {
  id: string
  label: string
  icon: ReactNode
  active: boolean
  onToggle: () => void
}

interface SearchPopoverProps {
  value: string
  onChange: (value: string) => void
  onKeyDown?: (event: React.KeyboardEvent<HTMLInputElement>) => void
  onClose: () => void
  placeholder: string
  /** Stable `name` for the search input (a11y: form fields need an id or name). */
  inputName?: string
  inputRef?: RefObject<HTMLInputElement | null>
  matchLabel?: string | null
  matchTone?: 'default' | 'warning'
  onNext?: () => void
  onPrevious?: () => void
  canNavigate?: boolean
  options?: SearchToggleOption[]
  leadingControl?: ReactNode
  extraActions?: ReactNode
  secondaryRow?: ReactNode
  className?: string
}

const searchSurfaceVariants = cva(
  'w-[320px] rounded-xl border border-border/70 bg-background/95 p-1.5 shadow-[0_16px_36px_-28px_rgba(0,0,0,0.55)] backdrop-blur-sm',
)

const searchIconButtonVariants = cva(
  'flex size-6 items-center justify-center rounded-lg border border-transparent text-muted-foreground transition-colors hover:border-border/70 hover:bg-muted hover:text-foreground',
  {
    variants: {
      disabled: {
        true: 'cursor-not-allowed opacity-50',
        false: '',
      },
    },
    defaultVariants: {
      disabled: false,
    },
  },
)

const searchToggleButtonVariants = cva(
  'flex size-6 items-center justify-center rounded-lg border border-transparent transition-colors hover:border-border/70 hover:bg-muted',
  {
    variants: {
      active: {
        true: 'border-border/70 bg-muted text-foreground',
        false: 'text-muted-foreground',
      },
    },
    defaultVariants: {
      active: false,
    },
  },
)

const searchActionButtonVariants = cva(
  'ui-font ui-text-sm flex h-8 items-center justify-center rounded-lg border border-transparent px-2.5 text-muted-foreground transition-colors hover:border-border/70 hover:bg-muted hover:text-foreground',
  {
    variants: {
      disabled: {
        true: 'cursor-not-allowed opacity-50',
        false: '',
      },
    },
    defaultVariants: {
      disabled: false,
    },
  },
)

export function SearchPopover({
  value,
  onChange,
  onKeyDown,
  onClose,
  placeholder,
  inputName,
  inputRef,
  matchLabel,
  matchTone = 'default',
  onNext,
  onPrevious,
  canNavigate = true,
  options = [],
  leadingControl,
  extraActions,
  secondaryRow,
  className,
}: SearchPopoverProps) {
  return (
    <div data-oracle-id="search-popover" className={cn(searchSurfaceVariants(), className)}>
      <div className="flex items-center gap-1.5">
        {leadingControl}

        <div className="relative min-w-0 flex-1">
          <Search className="-translate-y-1/2 pointer-events-none absolute top-1/2 left-2.5 text-muted-foreground" />
          <Input
            ref={inputRef}
            type="text"
            name={inputName}
            value={value}
            onChange={(event) => onChange(event.target.value)}
            onKeyDown={onKeyDown}
            placeholder={placeholder}
            className="ui-text-sm h-8 rounded-lg border-border/80 bg-background py-1 pr-8 pl-8"
          />
          {value && (
            <Button
              type="button"
              onClick={() => onChange('')}
              variant="ghost"
              compact
              className="-translate-y-1/2 absolute top-1/2 right-1"
              aria-label="Clear search"
            >
              <X />
            </Button>
          )}
        </div>

        {matchLabel && (
          <span
            className={cn(
              'ui-font ui-text-sm shrink-0',
              matchTone === 'warning' ? 'text-amber-400' : 'text-muted-foreground',
            )}
          >
            {matchLabel}
          </span>
        )}

        {extraActions}

        <Button
          type="button"
          onClick={onClose}
          variant="ghost"
          className={searchIconButtonVariants()}
          aria-label="Close search"
          data-oracle-id="search-close"
          compact
        >
          <X />
        </Button>
      </div>

      {(options.length > 0 || onPrevious || onNext) && (
        <div className="mt-1.5 flex items-center justify-between gap-2">
          <div className="flex items-center gap-1">
            {options.map((option) => (
              <Button
                key={option.id}
                type="button"
                onClick={option.onToggle}
                variant="ghost"
                className={searchToggleButtonVariants({
                  active: option.active,
                })}
                tooltip={option.label}
                aria-label={option.label}
                aria-pressed={option.active}
                data-oracle-id={`search-toggle-${option.id}`}
                compact
              >
                {option.icon}
              </Button>
            ))}
          </div>

          {(onPrevious || onNext) && (
            <div className="flex items-center gap-1">
              {onPrevious && (
                <Button
                  type="button"
                  onClick={onPrevious}
                  disabled={!canNavigate}
                  variant="ghost"
                  className={searchIconButtonVariants({
                    disabled: !canNavigate,
                  })}
                  aria-label="Previous match"
                  data-oracle-id="search-nav-previous"
                  compact
                >
                  <ChevronUp />
                </Button>
              )}
              {onNext && (
                <Button
                  type="button"
                  onClick={onNext}
                  disabled={!canNavigate}
                  variant="ghost"
                  className={searchIconButtonVariants({
                    disabled: !canNavigate,
                  })}
                  aria-label="Next match"
                  data-oracle-id="search-nav-next"
                  compact
                >
                  <ChevronDown />
                </Button>
              )}
            </div>
          )}
        </div>
      )}

      {secondaryRow && <div className="mt-1.5">{secondaryRow}</div>}
    </div>
  )
}

export function SearchReplaceToggle({
  isExpanded,
  onToggle,
  expandedLabel = 'Hide replace',
  collapsedLabel = 'Show replace',
}: {
  isExpanded: boolean
  onToggle: () => void
  expandedLabel?: string
  collapsedLabel?: string
}) {
  const label = isExpanded ? expandedLabel : collapsedLabel

  return (
    <Button
      type="button"
      onClick={onToggle}
      variant="ghost"
      className={searchIconButtonVariants()}
      tooltip={label}
      aria-label={label}
      data-oracle-id="search-replace-toggle"
      compact
    >
      <ChevronRight className={cn('transition-transform', isExpanded && 'rotate-90')} />
    </Button>
  )
}

export function SearchReplaceRow({
  value,
  onChange,
  onKeyDown,
  inputRef,
  onReplace,
  onReplaceAll,
  canReplace,
  canReplaceAll = canReplace,
  replaceAllTooltip,
}: {
  value: string
  onChange: (value: string) => void
  onKeyDown?: (event: React.KeyboardEvent<HTMLInputElement>) => void
  inputRef?: RefObject<HTMLInputElement | null>
  onReplace: () => void
  onReplaceAll: () => void
  canReplace: boolean
  canReplaceAll?: boolean
  replaceAllTooltip?: string
}) {
  return (
    <div
      data-oracle-id="search-replace-row"
      className="flex items-center gap-1.5 border-border/60 border-t pt-1.5"
    >
      <span
        data-oracle-id="search-replace-icon"
        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border/70 bg-background text-muted-foreground"
      >
        <Replace />
      </span>

      <Input
        ref={inputRef}
        type="text"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={onKeyDown}
        placeholder="Replace with..."
        className="ui-text-sm h-8 flex-1 rounded-lg border-border/80 bg-background py-1"
      />

      <Button
        type="button"
        onClick={onReplace}
        disabled={!canReplace}
        variant="ghost"
        className={searchActionButtonVariants({ disabled: !canReplace })}
        data-oracle-id="search-replace-confirm"
        data-oracle-content-sized="true"
        compact
      >
        Replace
      </Button>
      <Button
        type="button"
        onClick={onReplaceAll}
        disabled={!canReplaceAll}
        variant="ghost"
        className={searchActionButtonVariants({ disabled: !canReplaceAll })}
        tooltip={replaceAllTooltip}
        data-oracle-id="search-replace-all"
        data-oracle-content-sized="true"
        compact
      >
        All
      </Button>
    </div>
  )
}
