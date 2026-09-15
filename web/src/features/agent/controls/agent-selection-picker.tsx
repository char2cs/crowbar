import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { MagnifyingGlass as Search } from '@phosphor-icons/react'
import { Dropdown } from '@/components/ui/dropdown'
import { Input } from '@/components/ui/input'
import { ProviderIcon } from '@/components/ui/provider-icon'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import { CheckIcon, UpDownIcon } from '@/features/agent/shared/agent-icons'
import { cn } from '@/lib/utils'

// The menu's width. Wide enough for a provider section header plus its
// longest model id (`gpt-5.6-terra`) without wrapping. An inline STYLE, not a
// class: Dropdown locks its measured content width on open and would
// overwrite a class-set one (see provider-picker.tsx's own MENU_WIDTH_PX).
const MENU_WIDTH_PX = 280

export interface AgentSelectionPickerProps {
  /** The agent running this chat, or undefined before any runner has been on it. */
  provider?: AgentProvider
  providers: AgentProvider[]
  /** The chat's EFFECTIVE model — its sticky selection, or a staged pick on
   *  top of it. '' means unset. */
  model: string
  effort: string
  /** A turn is in flight, or a switch is already running. */
  disabled?: boolean
  /** A pick — LOCAL staging only, provider included. Nothing is written to
   *  the server and no CLI is switched until the next message actually
   *  sends; see AgentChatPane's `stageSelection`. Picking a model under a
   *  DIFFERENT provider's section stages that provider too — it never calls
   *  a switch itself, so a row click can never tear down the live CLI on its
   *  own. */
  onSelectionChange: (provider: string, model: string, effort: string) => void
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

// Cheap subsequence fuzzy score: every query char must appear in order;
// contiguous runs score higher so "hgh" ranks "high" over "haiku".
function fuzzyScore(query: string, text: string): number {
  if (!query) return 1
  const q = query.toLowerCase()
  const t = text.toLowerCase()
  let qi = 0
  let score = 0
  let streak = 0
  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) {
      qi++
      streak++
      score += streak
    } else {
      streak = 0
    }
  }
  return qi === q.length ? score + 1 : 0
}

function effortLabel(level: string): string {
  return level === 'xhigh' ? 'XHigh' : level.charAt(0).toUpperCase() + level.slice(1)
}

/** Which effort levels are valid for `model` on `provider` — a property of
 *  the MODEL, never the provider alone (AgentModelPicker's own rule). */
function effortLevelsFor(provider: AgentProvider | undefined, model: string): string[] {
  if (!provider?.effortSelect) return []
  return provider.efforts?.[model] ?? []
}

/** The effort to land on after picking `model`: keep the current one if the
 *  new model still declares it, otherwise that model's own first level. There
 *  is no "provider default" row in this picker (unlike the two single-axis
 *  dropdowns it replaces) — every row lands on a concrete, visible value. */
function resolveEffort(provider: AgentProvider, model: string, currentEffort: string): string {
  const levels = effortLevelsFor(provider, model)
  if (currentEffort && levels.includes(currentEffort)) return currentEffort
  return levels[0] ?? ''
}

/**
 * The chat's merged provider + model + effort control.
 *
 * One trigger and one menu instead of three chip dropdowns: picking a MODEL
 * already picks its PROVIDER (each provider is a section, its models the
 * rows under it — there is no separate provider row to choose first), and
 * effort is a property of whichever model is current, so it lives as its own
 * persistent control at the foot of the menu — the last thing you'd touch,
 * not one row among many to search past.
 *
 * Absent capability, absent UI, same house rule as the pickers this replaces:
 * a provider with no model catalogue contributes no section, and a chat with
 * NO model-select provider at all gets no control here — never a disabled one.
 */
export function AgentSelectionPicker({
  provider,
  providers,
  model,
  effort,
  disabled,
  onSelectionChange,
}: AgentSelectionPickerProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [highlight, setHighlight] = useState(0)
  const anchorRef = useRef<HTMLButtonElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)

  // Offered = installed AND enabled — the same filter AgentProviderPicker
  // applied: a provider whose CLI isn't on PATH cannot be spawned, and one
  // the user switched off in Settings asked not to be.
  const offered = useMemo(
    () => providers.filter((candidate) => candidate.connected && candidate.enabled),
    [providers],
  )

  // Whether there is anything to pick AT ALL — independent of the search
  // query, which can legitimately filter every row out while a query is
  // stale from a closed-without-picking menu. This, not `sections`, is what
  // decides whether the trigger renders.
  const catalogueProviders = useMemo(
    () =>
      offered.filter((candidate) => candidate.modelSelect && (candidate.models?.length ?? 0) > 0),
    [offered],
  )

  const sections = useMemo<ProviderSection[]>(() => {
    return catalogueProviders
      .map((candidate) => {
        // A query matching the PROVIDER's own name surfaces every model
        // under it, same as T3code's own model picker treats a provider
        // match — typing "codex" should not require also knowing a model name.
        const sectionScore = fuzzyScore(query, candidate.displayName)
        const rows = (candidate.models ?? [])
          .map((m) => ({
            model: m,
            picked: candidate.id === provider?.id && m === model,
            score: sectionScore > 0 ? Math.max(sectionScore, 1) : fuzzyScore(query, m),
          }))
          .filter((row) => row.score > 0)
        return { provider: candidate, rows }
      })
      .filter((section) => section.rows.length > 0)
  }, [catalogueProviders, provider?.id, model, query])

  const flatRows = useMemo(
    () => sections.flatMap((section) => section.rows.map((row) => ({ section, row }))),
    [sections],
  )

  const levels = effortLevelsFor(provider, model)
  const effortIndex = Math.max(0, levels.indexOf(effort))
  const effortPct = levels.length > 1 ? Math.round((effortIndex / (levels.length - 1)) * 100) : 0

  useEffect(() => {
    if (!isOpen) return
    setQuery('')
    setHighlight(0)
    const frame = window.requestAnimationFrame(() => searchRef.current?.focus())
    return () => window.cancelAnimationFrame(frame)
  }, [isOpen])

  const close = useCallback(() => setIsOpen(false), [])

  // Picking a model under a DIFFERENT provider's section stages that
  // provider right alongside it — never a live SwitchProvider call. The
  // picker only ever stages; see AgentSelectionPickerProps.onSelectionChange.
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

  // A real slider: press ANYWHERE on the track (thumb included — it sits
  // over the track and is otherwise non-interactive) captures the pointer,
  // jumps to that position immediately, and keeps tracking every move until
  // release, snapping continuously to the nearest discrete level. Not just a
  // click-to-jump bar with clickable tick labels underneath.
  const setEffortFromClientX = useCallback(
    (track: HTMLDivElement, clientX: number) => {
      if (levels.length === 0) return
      const rect = track.getBoundingClientRect()
      const ratio =
        rect.width === 0 ? 0 : Math.max(0, Math.min(1, (clientX - rect.left) / rect.width))
      const index = Math.round(ratio * (levels.length - 1))
      const nextLevel = levels[index]
      if (nextLevel) pickEffort(nextLevel)
    },
    [levels, pickEffort],
  )

  // Plain window listeners for the life of the gesture, not the Pointer
  // Capture API — confirmed live that this app's WKWebView throws
  // NotFoundError from setPointerCapture even for a genuine user pointerdown
  // (not just a synthetic one), which silently ate the value-jump on press
  // too, since it aborted the handler before setEffortFromClientX ran. This
  // needs no capture support at all: press sets the value and starts
  // tracking, move keeps tracking while the ref says so, release/cancel
  // stops it — the same contract capture would have given, without it.
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

  // No provider offers a model catalogue at all: absence, not a disabled
  // control — the same law every picker on this surface follows.
  if (catalogueProviders.length === 0) {
    return null
  }

  let rowIndex = -1

  return (
    <>
      <button
        ref={anchorRef}
        type="button"
        disabled={disabled}
        data-testid="agent-selection-picker"
        title={`${provider?.displayName ?? 'Agent'} — provider, model and effort.`}
        aria-label={`Agent: ${provider?.displayName ?? 'none'}, model ${model || 'unset'}, effort ${effort || 'unset'}`}
        aria-haspopup="menu"
        aria-expanded={isOpen}
        onClick={() => setIsOpen((open) => !open)}
        className={cn('chip max-w-56', (provider || model) && 'set')}
      >
        {provider && <ProviderIcon svg={provider.icon} className="size-3" />}
        {model && <b className="font-semibold text-foreground">{model}</b>}
        {model && effort && <span className="opacity-50">&middot;</span>}
        {effort && <span className="truncate">{effortLabel(effort)}</span>}
        {!model && <span className="truncate">{provider?.displayName ?? 'Agent'}</span>}
        <UpDownIcon size={12} className="opacity-55" />
      </button>
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
            {sections.length === 0 ? (
              <p className="px-2.5 py-6 text-center text-muted-foreground text-xs">
                No matches for &ldquo;{query}&rdquo;
              </p>
            ) : (
              sections.map((section) => (
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
                        onClick={() => pickModel(section.provider, row.model)}
                        onMouseEnter={() => setHighlight(rowIndex)}
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
              ))
            )}
          </div>
          {levels.length > 0 && (
            <div className="border-border/60 border-t px-2.5 pt-2.5 pb-2">
              <div className="mb-2.5 flex items-baseline justify-between text-muted-foreground text-xs">
                <span>Effort</span>
                <b className="font-semibold text-foreground">{effortLabel(effort)}</b>
              </div>
              <div className="px-1.5 py-2">
                <div
                  role="slider"
                  aria-label="Reasoning effort"
                  aria-valuemin={0}
                  aria-valuemax={levels.length - 1}
                  aria-valuenow={effortIndex}
                  aria-valuetext={effortLabel(effort)}
                  tabIndex={0}
                  onPointerDown={onTrackPointerDown}
                  onKeyDown={(event) => {
                    if (event.key === 'ArrowRight' || event.key === 'ArrowUp') {
                      event.preventDefault()
                      const next = levels[Math.min(effortIndex + 1, levels.length - 1)]
                      if (next) pickEffort(next)
                    } else if (event.key === 'ArrowLeft' || event.key === 'ArrowDown') {
                      event.preventDefault()
                      const prev = levels[Math.max(effortIndex - 1, 0)]
                      if (prev) pickEffort(prev)
                    }
                  }}
                  className="relative h-1 cursor-grab touch-none rounded-full bg-muted active:cursor-grabbing"
                >
                  <div
                    className="pointer-events-none absolute inset-y-0 left-0 rounded-full bg-primary transition-[width]"
                    style={{ width: `${effortPct}%` }}
                  />
                  <div
                    className="-translate-y-1/2 -translate-x-1/2 pointer-events-none absolute top-1/2 size-4 rounded-full border-2 border-primary bg-popover shadow-[0_1px_3px_oklch(0_0_0/28%)] transition-[left]"
                    style={{ left: `${effortPct}%` }}
                  />
                </div>
              </div>
              <div className="mt-2.5 flex justify-between">
                {levels.map((level) => (
                  <button
                    key={level}
                    type="button"
                    onClick={() => pickEffort(level)}
                    className={cn(
                      'rounded px-1 py-0.5 text-[10px] hover:bg-muted hover:text-foreground',
                      level === effort ? 'font-bold text-foreground' : 'text-muted-foreground',
                    )}
                  >
                    {effortLabel(level)}
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      </Dropdown>
    </>
  )
}
