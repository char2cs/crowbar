import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { MagnifyingGlass as Search } from '@phosphor-icons/react'
import { Dropdown } from '@/components/ui/dropdown'
import { Input } from '@/components/ui/input'
import { ProviderIcon } from '@/components/ui/provider-icon'
import type { AgentProvider, PermissionLevel } from '@/features/agent/api/agent-providers-api'
import {
  usePermissionLevel,
  type PermissionTarget,
} from '@/features/agent/composer/permission-level-switcher'
import { CheckIcon, UpDownIcon } from '@/features/agent/shared/agent-icons'
import { cn } from '@/lib/utils'
import { hasNativeChrome } from '@/lib/native-chrome'
import { IS_MOBILE } from '@/utils/platform'
import { effortLevelsFor, resolveEffort } from './agent-selection-efforts'
import { fuzzyScore } from './agent-selection-search'
import { NativeModelPicker } from './native-model-picker'

// Fits the longest model id unwrapped. An inline style because Dropdown locks
// its measured width on open and would overwrite a class-set one.
const MENU_WIDTH_PX = 280

export interface AgentSelectionPickerProps {
  /** The agent running this chat, or undefined before any runner has been on it. */
  provider?: AgentProvider
  providers: AgentProvider[]
  /** The effective model (sticky selection or staged pick); '' means unset. */
  model: string
  effort: string
  /** A turn is in flight, or a switch is already running. */
  disabled?: boolean
  /** Local staging only, provider included: nothing is written or switched
   *  until the next message sends, so a row click never tears down the live CLI. */
  onSelectionChange: (provider: string, model: string, effort: string) => void
  /** Display text for an unset `model`/`effort` only. Values stay raw elsewhere:
   *  `pickEffort` echoes `model` back out, and a label must never become a value. */
  unsetLabel?: string
  /** The model the last turn reported, shown beside an unset label. Display only:
   *  substituting it for `model` would break the `efforts['']` lookup. */
  reportedModel?: string
  /** On the phone the chat's permission level is chosen inside this picker, not in the footer. */
  permission?: PermissionTarget
}

interface ModelRow {
  model: string
  picked: boolean
  score: number
}

interface ProviderSection {
  provider: AgentProvider
  rows: ModelRow[]
}

function effortLabel(level: string): string {
  return level === 'xhigh' ? 'XHigh' : level.charAt(0).toUpperCase() + level.slice(1)
}

/** Search results, split out so the running rowIndex used for highlighting
 *  stays local. */
function ModelSections({
  sections,
  query,
  highlight,
  onHighlight,
  onPick,
}: {
  sections: ProviderSection[]
  query: string
  highlight: number
  onHighlight: (index: number) => void
  onPick: (provider: AgentProvider, model: string) => void
}) {
  if (sections.length === 0) {
    return (
      <p className="px-2.5 py-6 text-center text-muted-foreground text-xs">
        No matches for &ldquo;{query}&rdquo;
      </p>
    )
  }

  let rowIndex = -1
  return (
    <>
      {sections.map((section) => (
        <div key={section.provider.id}>
          <div className="ui-font ui-text-sm flex items-center gap-1.5 px-2.5 py-1 text-muted-foreground">
            <ProviderIcon svg={section.provider.icon} className="size-3" />
            <span>{section.provider.displayName}</span>
          </div>
          {section.rows.map((row) => {
            rowIndex += 1
            const isHighlighted = rowIndex === highlight
            return (
              <button
                key={row.model}
                type="button"
                role="menuitem"
                onClick={() => onPick(section.provider, row.model)}
                onMouseEnter={() => onHighlight(rowIndex)}
                className={cn(
                  'ui-font ui-text-sm flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-foreground transition-colors',
                  isHighlighted ? 'bg-muted' : 'hover:bg-muted',
                )}
              >
                {row.picked ? (
                  <CheckIcon size={12} className="shrink-0" />
                ) : (
                  <span className="inline-block size-3 shrink-0" />
                )}
                <span className="min-w-0 flex-1 truncate">{row.model}</span>
              </button>
            )
          })}
        </div>
      ))}
    </>
  )
}

/** Renders nothing when the model declares no levels — absence, not a disabled
 *  control. Owns its drag gesture so the picker carries none of that state. */
function EffortSlider({
  levels,
  effort,
  defaultEffort,
  onPick,
}: {
  levels: string[]
  effort: string
  defaultEffort: string
  onPick: (level: string) => void
}) {
  // An unknown effort draws no thumb rather than clamping to the first level,
  // which would look like a pick nobody made. '' runs at the stated default.
  const shown = effort || defaultEffort
  const shownLabel = effort ? effortLabel(effort) : `${effortLabel(defaultEffort)} (default)`
  const knownIndex = levels.indexOf(shown)
  const hasKnownEffort = knownIndex !== -1
  const effortIndex = Math.max(0, knownIndex)
  const effortPct = levels.length > 1 ? Math.round((effortIndex / (levels.length - 1)) * 100) : 0

  // Press anywhere on the track jumps there and keeps tracking until release,
  // snapping to the nearest discrete level.
  const setEffortFromClientX = useCallback(
    (track: HTMLDivElement, clientX: number) => {
      if (levels.length === 0) return
      const rect = track.getBoundingClientRect()
      const ratio =
        rect.width === 0 ? 0 : Math.max(0, Math.min(1, (clientX - rect.left) / rect.width))
      const index = Math.round(ratio * (levels.length - 1))
      const nextLevel = levels[index]
      if (nextLevel) onPick(nextLevel)
    },
    [levels, onPick],
  )

  // Window listeners instead of pointer capture: WKWebView throws NotFoundError
  // from setPointerCapture even on a genuine pointerdown.
  const draggingRef = useRef<(() => void) | null>(null)

  const onTrackPointerDown = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      if (levels.length === 0) return
      event.preventDefault()
      const track = event.currentTarget
      setEffortFromClientX(track, event.clientX)
      draggingRef.current?.()
      const onMove = (moveEvent: PointerEvent) => setEffortFromClientX(track, moveEvent.clientX)
      const stop = () => {
        window.removeEventListener('pointermove', onMove)
        window.removeEventListener('pointerup', stop)
        window.removeEventListener('pointercancel', stop)
        draggingRef.current = null
      }
      window.addEventListener('pointermove', onMove)
      window.addEventListener('pointerup', stop)
      window.addEventListener('pointercancel', stop)
      draggingRef.current = stop
    },
    [levels, setEffortFromClientX],
  )

  useEffect(() => () => draggingRef.current?.(), [])

  if (levels.length === 0) return null

  return (
    <div className="border-border/60 border-t px-2.5 pt-2.5 pb-2">
      <div className="mb-2.5 flex items-baseline justify-between text-muted-foreground text-xs">
        <span>Effort</span>
        <b className="font-semibold text-foreground">{shown ? shownLabel : ''}</b>
      </div>
      <div className="px-1.5 py-2">
        <div
          role="slider"
          aria-label="Reasoning effort"
          aria-valuemin={0}
          aria-valuemax={levels.length - 1}
          aria-valuenow={hasKnownEffort ? effortIndex : undefined}
          aria-valuetext={shown ? shownLabel : ''}
          tabIndex={0}
          onPointerDown={onTrackPointerDown}
          onKeyDown={(event) => {
            if (event.key === 'ArrowRight' || event.key === 'ArrowUp') {
              event.preventDefault()
              const next = levels[Math.min(effortIndex + 1, levels.length - 1)]
              if (next) onPick(next)
            } else if (event.key === 'ArrowLeft' || event.key === 'ArrowDown') {
              event.preventDefault()
              const prev = levels[Math.max(effortIndex - 1, 0)]
              if (prev) onPick(prev)
            }
          }}
          className="relative h-1 cursor-grab touch-none rounded-full bg-muted active:cursor-grabbing"
        >
          {hasKnownEffort && (
            <>
              <div
                className="pointer-events-none absolute inset-y-0 left-0 rounded-full bg-primary transition-[width]"
                style={{ width: `${effortPct}%` }}
              />
              <div
                className="-translate-y-1/2 -translate-x-1/2 pointer-events-none absolute top-1/2 size-4 rounded-full border-2 border-primary bg-popover shadow-[0_1px_3px_oklch(0_0_0/28%)] transition-[left]"
                style={{ left: `${effortPct}%` }}
              />
            </>
          )}
        </div>
      </div>
      <div className="mt-2.5 flex justify-between">
        {levels.map((level) => (
          <button
            key={level}
            type="button"
            onClick={() => onPick(level)}
            className={cn(
              'rounded px-1 py-0.5 text-[10px] hover:bg-muted hover:text-foreground',
              level === shown ? 'font-bold text-foreground' : 'text-muted-foreground',
            )}
          >
            {effortLabel(level)}
          </button>
        ))}
      </div>
    </div>
  )
}

/** The phone's web fallback for the native sheet's level section. */
function PermissionRadios({
  options,
  level,
  onPick,
}: {
  options: { value: PermissionLevel; label: string }[]
  level: PermissionLevel | ''
  onPick: (level: PermissionLevel) => void
}) {
  return (
    <div role="group" aria-label="Permission level" className="border-border/60 border-t p-1">
      <div className="ui-font ui-text-sm px-2.5 py-1 text-muted-foreground">Permission</div>
      {options.map((opt) => (
        <button
          key={opt.value}
          type="button"
          role="menuitemradio"
          aria-checked={opt.value === level}
          onClick={() => onPick(opt.value)}
          className="ui-font ui-text-sm flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-foreground hover:bg-muted"
        >
          {opt.value === level ? (
            <CheckIcon size={12} className="shrink-0" />
          ) : (
            <span className="inline-block size-3 shrink-0" />
          )}
          {opt.label}
        </button>
      ))}
    </div>
  )
}

/** One menu: picking a model picks its provider (providers are sections), and
 *  effort sits at the foot as a property of the current model. A provider with
 *  no catalogue contributes nothing — never a disabled control. */
export function AgentSelectionPicker({
  provider,
  providers,
  model,
  effort,
  disabled,
  onSelectionChange,
  unsetLabel = 'unset',
  reportedModel,
  permission,
}: AgentSelectionPickerProps) {
  const modelLabel = model || unsetLabel
  const effortLabelText = effort || unsetLabel
  const showsReport = !model && Boolean(reportedModel)
  const [isOpen, setIsOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [highlight, setHighlight] = useState(0)
  const anchorRef = useRef<HTMLButtonElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const level = usePermissionLevel(IS_MOBILE && isOpen && permission ? permission : null)
  const permissionChoice =
    level.options.length > 0 ? { ...level, onPick: level.setLevel } : undefined

  // An uninstalled CLI cannot spawn; a disabled one the user asked not to see.
  const offered = useMemo(
    () => providers.filter((candidate) => candidate.connected && candidate.enabled),
    [providers],
  )

  // Decides whether the trigger renders, independent of a query that may filter every row out.
  const catalogueProviders = useMemo(
    () =>
      offered.filter((candidate) => candidate.modelSelect && (candidate.models?.length ?? 0) > 0),
    [offered],
  )

  const sections = useMemo<ProviderSection[]>(() => {
    const result: ProviderSection[] = []
    for (const candidate of catalogueProviders) {
      // Matching the provider's name surfaces all its models.
      const sectionScore = fuzzyScore(query, candidate.displayName)
      const rows: ModelRow[] = []
      for (const m of candidate.models ?? []) {
        const score = sectionScore > 0 ? Math.max(sectionScore, 1) : fuzzyScore(query, m)
        if (score > 0) {
          rows.push({ model: m, picked: candidate.id === provider?.id && m === model, score })
        }
      }
      if (rows.length > 0) {
        result.push({ provider: candidate, rows })
      }
    }
    return result
  }, [catalogueProviders, provider?.id, model, query])

  const flatRows = useMemo(
    () => sections.flatMap((section) => section.rows.map((row) => ({ section, row }))),
    [sections],
  )

  const levels = effortLevelsFor(provider, model)

  useEffect(() => {
    if (!isOpen) return
    setQuery('')
    setHighlight(0)
    const frame = window.requestAnimationFrame(() => searchRef.current?.focus())
    return () => window.cancelAnimationFrame(frame)
  }, [isOpen])

  const close = useCallback(() => setIsOpen(false), [])

  // Stages the provider alongside the model — never a live switch.
  const pickModel = useCallback(
    (targetProvider: AgentProvider, targetModel: string) => {
      const nextEffort = resolveEffort(targetProvider, targetModel, effort)
      onSelectionChange(targetProvider.id, targetModel, nextEffort)
      close()
    },
    [onSelectionChange, effort, close],
  )

  const pickEffort = useCallback(
    (level: string) => onSelectionChange(provider?.id ?? '', model, level),
    [provider?.id, model, onSelectionChange],
  )

  const onSearchKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLInputElement>) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        close()
      } else if (event.key === 'ArrowDown') {
        event.preventDefault()
        setHighlight((current) => Math.min(current + 1, Math.max(0, flatRows.length - 1)))
      } else if (event.key === 'ArrowUp') {
        event.preventDefault()
        setHighlight((current) => Math.max(current - 1, 0))
      } else if (event.key === 'Enter') {
        event.preventDefault()
        const target = flatRows[Math.max(0, Math.min(highlight, flatRows.length - 1))]
        if (target) pickModel(target.section.provider, target.row.model)
      }
    },
    [flatRows, highlight, close, pickModel],
  )

  if (catalogueProviders.length === 0) {
    return null
  }

  return (
    <>
      <button
        ref={anchorRef}
        type="button"
        disabled={disabled}
        data-testid="agent-selection-picker"
        title={`${provider?.displayName ?? 'Agent'} — provider, model and effort.`}
        aria-label={`Agent: ${provider?.displayName ?? 'none'}, model ${modelLabel}${showsReport ? ` (reported ${reportedModel})` : ''}, effort ${effortLabelText}`}
        aria-haspopup="menu"
        aria-expanded={isOpen}
        onClick={() => setIsOpen((open) => !open)}
        className={cn('chip max-w-56', (provider || model) && 'set')}
      >
        {provider && <ProviderIcon svg={provider.icon} className="size-3" />}
        <b className="font-semibold text-foreground">{modelLabel}</b>
        {showsReport && (
          <span className="truncate text-[11px] text-muted-foreground">({reportedModel})</span>
        )}
        {(model || effort) && (
          <>
            <span className="opacity-50">&middot;</span>
            <span className="truncate">{effortLabel(effortLabelText)}</span>
          </>
        )}
        <UpDownIcon size={12} className="opacity-55" />
      </button>
      {hasNativeChrome() ? (
        <NativeModelPicker
          open={isOpen}
          providers={catalogueProviders}
          provider={provider}
          model={model}
          effort={effort}
          permission={permissionChoice}
          onSelectionChange={onSelectionChange}
          onClose={close}
        />
      ) : (
        <Dropdown
          isOpen={isOpen}
          onClose={close}
          anchorRef={anchorRef}
          anchorSide="top"
          anchorAlign="start"
          style={{ width: MENU_WIDTH_PX }}
        >
          <div className="flex max-h-100 flex-col">
            <div className="border-border/60 border-b px-1.5 pb-1.5 pt-0.5">
              <Input
                ref={searchRef}
                type="text"
                placeholder="Search models&hellip;"
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value)
                  setHighlight(0)
                }}
                onKeyDown={onSearchKeyDown}
                leftIcon={Search}
                size="sm"
              />
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto p-1">
              <ModelSections
                sections={sections}
                query={query}
                highlight={highlight}
                onHighlight={setHighlight}
                onPick={pickModel}
              />
            </div>
            <EffortSlider
              levels={levels}
              effort={effort}
              defaultEffort={provider?.defaultEfforts?.[model] ?? ''}
              onPick={pickEffort}
            />
            {permissionChoice && (
              <PermissionRadios
                options={permissionChoice.options}
                level={permissionChoice.level}
                onPick={permissionChoice.onPick}
              />
            )}
          </div>
        </Dropdown>
      )}
    </>
  )
}
