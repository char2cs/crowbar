import { useRef, useState } from 'react'
import { CaretUpDown, Check } from '@phosphor-icons/react'
import { Dropdown, dropdownTriggerClassName, type MenuItem } from '@/components/ui/dropdown'
import { setChatSelection, type AgentProvider } from '@/features/agent/api/agent-api'
import { ApiError } from '@/lib/api'
import { cn } from '@/lib/utils'

// The menu's width. Model ids are long-ish (`gpt-5.6-terra`) and effort levels are
// one word, but both menus share the number so the two controls cannot drift into
// looking like unrelated things stacked by accident. It is an inline STYLE, not a
// class: Dropdown locks its measured content width on open and would overwrite a
// class-set width with it (see provider-switch-dropdown).
const MENU_WIDTH_PX = 180

// The menu row that goes BACK to the provider's own default, and the accessible
// name a trigger carries when the chat has made no choice. Deliberately NOT the
// name of any model: unset means "whatever this provider defaults to", Crowbar
// does not know what that resolves to, and naming one would be a fabrication.
const DEFAULT_LABEL = 'Provider default'

export interface AgentModelPickerProps {
  wsId: string
  chatId: string
  /** The chat's provider, or undefined for a chat no runner has ever been on. */
  provider?: AgentProvider
  /** The chat's sticky selection. '' means unset — the provider's own default. */
  model: string
  effort: string
  /**
   * Commit an ACCEPTED selection upward. The endpoint answers 202 with no body and
   * no lifecycle frame follows it, so the caller owning the chat is what keeps the
   * store true; without it the control would paint from a value the server has
   * already moved past.
   */
  onSelectionChange: (model: string, effort: string) => void
}

/**
 * The chat's model and reasoning-effort picker.
 *
 * Three rules do most of the work here:
 *
 *  1. A provider that declares no catalogue gets NO CONTROL — not a disabled one.
 *     A greyed-out picker says "broken"; absence says "this provider does not
 *     offer that", which is the truth.
 *
 *  2. NOTHING is shown as selected until the chat has actually chosen. The
 *     provider's default is real and unnamed, so the first catalogue entry is not
 *     a stand-in for it, and `Provider default` is offered as an explicit way back
 *     (it sends '', which is how the endpoint clears a half).
 *
 *  3. Effort levels are a property of the MODEL. Switching model re-validates the
 *     effort against `efforts[newModel]` and clears it IN THE SAME WRITE when the
 *     new model does not declare it — codex's gpt-5.6-sol goes up to `ultra` while
 *     gpt-5.6-luna stops at `max`, so moving between them can otherwise strand a
 *     level that pair never jointly supported.
 *
 * The catalogue itself is read straight off the provider: `efforts[model]`, with
 * '' as the key when nothing is selected. The backend has already applied the
 * descriptor's model-independent fallback, so there is no fallback rule here and
 * no provider knowledge hardcoded.
 */
export function AgentModelPicker({
  wsId,
  chatId,
  provider,
  model,
  effort,
  onSelectionChange,
}: AgentModelPickerProps) {
  // The pair being written. It exists so the control paints the new choice the
  // instant it is made instead of waiting a round trip, and it is dropped whether
  // the write succeeds (the parent's value has arrived) or fails (the old one
  // stands, under an error that says why).
  const [pending, setPending] = useState<{ model: string; effort: string } | null>(null)
  const [error, setError] = useState('')

  const shownModel = pending?.model ?? model
  const shownEffort = pending?.effort ?? effort

  const models = provider?.modelSelect ? (provider.models ?? []) : []
  const levelsFor = (id: string) => (provider?.effortSelect ? (provider.efforts?.[id] ?? []) : [])
  const levels = levelsFor(shownModel)

  const commit = async (nextModel: string, nextEffort: string) => {
    setError('')
    setPending({ model: nextModel, effort: nextEffort })
    try {
      await setChatSelection(wsId, chatId, nextModel, nextEffort)
      onSelectionChange(nextModel, nextEffort)
    } catch (failure) {
      setError(selectionErrorMessage(failure))
    } finally {
      setPending(null)
    }
  }

  // A model with no levels is ABSENT from the map, so this clears the effort for
  // exactly the same reason it clears an unsupported one: the pair has to be
  // jointly valid before it is sent, not after it is rejected.
  const pickModel = (next: string) =>
    void commit(next, shownEffort && levelsFor(next).includes(shownEffort) ? shownEffort : '')

  const pickEffort = (next: string) => void commit(shownModel, next)

  // Neither capability, nothing to draw. This is the absent-UI rule, and it also
  // covers a chat whose provider is not known yet.
  if (models.length === 0 && levels.length === 0) return null

  // A selection the CURRENT provider does not declare. Switching provider mid-chat
  // keeps the chat's sticky choice — the two are independent — so a chat moved from
  // claude to codex can be holding `sonnet`, which codex has never heard of. The
  // trigger goes on naming what the chat actually asked for, because that is the
  // truth; this line is what stops that truth from looking like a working choice.
  const stranded =
    (shownModel !== '' && models.length > 0 && !models.includes(shownModel)) ||
    (shownEffort !== '' && levels.length > 0 && !levels.includes(shownEffort))

  // Switching is not destructive and must not be dressed as if it were: the CLI is
  // restarted into the SAME conversation, which it resumes.
  const hint =
    `Applies to your next message: ${provider?.displayName ?? 'the provider'}'s CLI ` +
    'restarts and resumes this conversation.'

  return (
    <div className="flex min-w-0 flex-col items-start gap-1" data-testid="agent-model-picker">
      <div className="flex min-w-0 items-center gap-1">
        {models.length > 0 && (
          <SelectionDropdown
            axis="Model"
            value={shownModel}
            options={models}
            hint={hint}
            busy={pending !== null}
            onPick={pickModel}
          />
        )}
        {levels.length > 0 && (
          <SelectionDropdown
            axis="Effort"
            value={shownEffort}
            options={levels}
            hint={hint}
            busy={pending !== null}
            onPick={pickEffort}
          />
        )}
      </div>
      {stranded && !error && (
        <p className="text-muted-foreground text-xs">
          {provider?.displayName ?? 'This provider'} does not offer that. Pick one it declares, or
          go back to its default.
        </p>
      )}
      {error && (
        <p className="text-destructive text-xs" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}

function SelectionDropdown({
  axis,
  value,
  options,
  hint,
  busy,
  onPick,
}: {
  axis: string
  /** '' when the chat has chosen nothing — shown as the default, never as a name. */
  value: string
  options: string[]
  hint: string
  busy: boolean
  onPick: (value: string) => void
}) {
  const [isOpen, setIsOpen] = useState(false)
  const anchorRef = useRef<HTMLButtonElement>(null)
  // text-xs, not the chrome's ui-text-xs: this sits on the chat surface beside the
  // context gauge, and the two have to read as one line.

  // The default row leads, then the catalogue in DESCRIPTOR ORDER — the provider's
  // own ranking, never re-sorted. The tick marks what the chat chose, and on the
  // default row it marks the absence of a choice rather than a value.
  const tick = <Check size={12} data-testid="selection-tick" />
  const items: MenuItem[] = [
    {
      id: 'provider-default',
      label: DEFAULT_LABEL,
      icon: value === '' ? tick : undefined,
      onClick: () => onPick(''),
    },
    { id: 'separator', label: '', separator: true, onClick: () => {} },
    ...options.map((option) => ({
      id: option,
      label: option,
      icon: value === option ? tick : undefined,
      onClick: () => onPick(option),
    })),
  ]

  return (
    <>
      <button
        ref={anchorRef}
        type="button"
        disabled={busy}
        title={hint}
        aria-label={`${axis}: ${value || DEFAULT_LABEL}`}
        aria-haspopup="menu"
        aria-expanded={isOpen}
        onClick={() => setIsOpen((open) => !open)}
        className={dropdownTriggerClassName('h-6 max-w-40 justify-between text-xs', 'ghost')}
      >
        {/* Unset reads as "Default model" / "Default effort" rather than the menu's
            "Provider default": side by side, two triggers saying the same words
            would be one control the eye cannot split in half. */}
        <span className={cn('truncate', value ? 'text-foreground' : 'text-muted-foreground')}>
          {value || `Default ${axis.toLowerCase()}`}
        </span>
        <CaretUpDown size={12} className="shrink-0 text-muted-foreground" />
      </button>
      {/* Always mounted, never gated on `busy`. Picking an item is exactly what
          sets `busy`, so unmounting on it would tear the menu out mid-exit and
          the close would snap where every other menu in the app eases. The
          trigger's `disabled` is what prevents a second write. */}
      <Dropdown
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        anchorRef={anchorRef}
        anchorSide="top"
        anchorAlign="start"
        className="min-w-0"
        style={{ width: MENU_WIDTH_PX }}
        items={items}
      />
    </>
  )
}

/**
 * Say what the server said, in the user's terms.
 *
 * The two statuses this endpoint answers with are both actionable and neither is a
 * bug to hide: 400 is a value the provider no longer declares (a catalogue that
 * moved under a stored choice), 422 is a chat that has no provider to configure
 * yet.
 */
function selectionErrorMessage(failure: unknown): string {
  if (failure instanceof ApiError) {
    if (failure.status === 400) {
      return 'This provider no longer offers that combination. Pick another, or go back to its default.'
    }
    if (failure.status === 422) {
      return 'This chat has no provider yet. Start it, then choose a model.'
    }
  }
  return failure instanceof Error ? failure.message : String(failure)
}
