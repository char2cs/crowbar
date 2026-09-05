import { useRef, useState } from 'react'
import { CaretUpDown } from '@phosphor-icons/react'
import { Dropdown, dropdownTriggerClassName } from '@/components/ui/dropdown'
import { ProviderIcon } from '@/components/ui/provider-icon'
import type { AgentProvider } from '@/features/agent/api/agent-api'

// The trigger's width AND its menu's — one number so they cannot drift apart. The shared
// dropdown's 240px default ballooned the menu far wider than the trigger beneath it;
// matching the menu to the trigger instead shrank both to ~100px. This is the sane
// middle: wide enough for a provider row, narrow enough not to loom over the chat.
//
// The menu takes it as an inline STYLE, not a class, and that is deliberate. Dropdown
// locks its measured content width on open (applyLockedWidth) and would overwrite a
// class-set width with it — the menu silently rendered 4px narrower than the trigger.
// A style.width is what Dropdown treats as explicit and leaves alone.
const SWITCHER_WIDTH_PX = 180
// Must equal SWITCHER_WIDTH_PX. It cannot be interpolated: Tailwind only generates
// classes it can see as literals in the source.
const SWITCHER_WIDTH_CLASS = 'w-[180px]'

export interface ProviderSwitchDropdownProps {
  providers: AgentProvider[]
  currentProviderId: string
  onSwitch: (providerId: string) => void
  disabled?: boolean
}

// Chat-pane footer control (Task 15 places it): trigger shows the chat's
// current provider, menu lists the OTHER providers to switch to.
export function ProviderSwitchDropdown({
  providers,
  currentProviderId,
  onSwitch,
  disabled = false,
}: ProviderSwitchDropdownProps) {
  const [isOpen, setIsOpen] = useState(false)
  const anchorRef = useRef<HTMLButtonElement>(null)

  // `current` is looked up across ALL providers, disabled included: disabling a
  // provider never touches chats already running on it, so this trigger must keep
  // naming what the chat is actually talking to.
  //
  // `others` — the switch TARGETS — drops disabled providers. This menu offers a
  // provider the same way the New-chat rows do, and switching to a disabled one
  // would spawn the very CLI the user turned off.
  const current = providers.find((p) => p.id === currentProviderId)
  const others = providers.filter((p) => p.id !== currentProviderId && p.enabled)

  // Derived, not stored: a disabled provider closes its menu the instant it goes
  // disabled, with no render where a disabled trigger still shows an open menu.
  // isOpen itself stays true underneath (untouched by disabled going false again)
  // so re-enabling never surprises the user with a menu that silently reappeared.
  const open = isOpen && !disabled

  // Both menu shapes below (switch targets, or the nothing-to-switch-to hint)
  // share these, so the menu can never drift from its trigger. Dropdown's content
  // props are a discriminated union — `items` and `children` are mutually
  // exclusive — which is why this is a spread plus one explicit content prop
  // rather than a single element with a conditional child.
  const menuProps = {
    isOpen: open,
    onClose: () => setIsOpen(false),
    anchorRef,
    anchorSide: 'top' as const,
    anchorAlign: 'end' as const,
    className: 'min-w-0',
    style: { width: SWITCHER_WIDTH_PX },
  }

  return (
    <>
      {/* Ghost: the chat pane is one flat surface, and a filled pill floating beneath
          the terminal read as a stray button rather than part of the chat.
          The negative margin is load-bearing, not a nudge. A ghost trigger has no
          visible box, so its optical edge is its CONTENT, and the content sits behind
          9px of chrome: 8px padding PLUS the button's 1px transparent border, which
          renders invisible but still takes space. -mr, not -ml, because the switcher
          lives at the RIGHT end of the status line now: the edge that has to land on
          the terminal's last column is the caret's, not the icon's. Measured in the
          running app. The hover background still covers the full padded width. */}
      {/* The trigger and its menu are ONE control, so they share one width. Both are
          SWITCHER_WIDTH; anything else and they read as two unrelated things stacked by
          accident. justify-between is what makes the extra width usable: buttonVariants
          CENTRES its content, so widening the button alone would drift the icon into the
          middle and undo the column alignment above. Provider name left, caret right —
          a select. */}
      <button
        ref={anchorRef}
        type="button"
        disabled={disabled}
        title={disabled ? 'Wait for prompt delivery before switching providers' : undefined}
        onClick={() => setIsOpen((open) => !open)}
        className={dropdownTriggerClassName(
          `-mr-[9px] h-7 ${SWITCHER_WIDTH_CLASS} shrink-0 justify-between text-foreground`,
          'ghost',
        )}
      >
        <span className="flex min-w-0 items-center gap-1.5">
          {current && <ProviderIcon svg={current.icon} />}
          <span className="truncate">{current?.displayName ?? 'Provider'}</span>
        </span>
        <CaretUpDown size={12} className="shrink-0 text-muted-foreground" />
      </button>
      {/* menuProps carries the positioning and the shared width (anchorAlign="end"
          keeps the menu on the trigger's column; min-w-0 clears the root's 240px
          floor so the style width wins). See SWITCHER_WIDTH_PX. */}
      {!disabled && others.length > 0 ? (
        <Dropdown
          {...menuProps}
          items={others.map((provider) => ({
            id: provider.id,
            label: provider.displayName,
            icon: <ProviderIcon svg={provider.icon} />,
            onClick: () => onSwitch(provider.id),
          }))}
        />
      ) : !disabled ? (
        <Dropdown {...menuProps}>
          {/* A menu that opens onto nothing is a dead end. Say why it is empty and
              what a second provider would buy — above all that the conversation
              carries over, which is the part nobody would guess. Deliberately not a
              menuitem: there is nothing here to select. */}
          <p className="ui-font ui-text-xs px-2 py-1.5 text-muted-foreground">
            No other provider is enabled. Turn one on in Settings to switch mid-chat — this
            conversation&apos;s context comes with you.
          </p>
        </Dropdown>
      ) : null}
    </>
  )
}
