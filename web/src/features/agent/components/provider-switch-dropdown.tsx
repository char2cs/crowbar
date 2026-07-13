import { useRef, useState } from 'react'
import { CaretUpDown } from '@phosphor-icons/react'
import { Dropdown, dropdownTriggerClassName } from '@/components/ui/dropdown'
import type { AgentProvider } from '@/features/agent/api/agent-api'

// Inline the backend-trusted SVG markup so its `fill="currentColor"` paths
// inherit the ambient text-* theme token from whichever button/row hosts it —
// an <img src="..."> can't inherit currentColor or size to its container the
// same way. Mirrors FlickerSpinner's approach (components/ui/flicker-spinner.tsx).
function ProviderIcon({ svg }: { svg: string }) {
  return (
    <span
      aria-hidden="true"
      className="inline-flex size-3.5 shrink-0 items-center justify-center [&>svg]:size-full"
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  )
}

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
}

// Chat-pane footer control (Task 15 places it): trigger shows the chat's
// current provider, menu lists the OTHER providers to switch to.
export function ProviderSwitchDropdown({
  providers,
  currentProviderId,
  onSwitch,
}: ProviderSwitchDropdownProps) {
  const [isOpen, setIsOpen] = useState(false)
  const anchorRef = useRef<HTMLButtonElement>(null)

  const current = providers.find((p) => p.id === currentProviderId)
  const others = providers.filter((p) => p.id !== currentProviderId)

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
      <Dropdown
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        anchorRef={anchorRef}
        anchorSide="top"
        // The trigger now sits at the right end of the status line, so the menu hangs
        // from its RIGHT edge. (With both at SWITCHER_WIDTH_PX this is currently a
        // no-op, but it is what keeps the menu on the column if either width changes.)
        anchorAlign="end"
        // min-w-0 clears the root's 240px floor; the width itself comes from style so
        // Dropdown's width-lock does not overwrite it. See SWITCHER_WIDTH_PX.
        className="min-w-0"
        style={{ width: SWITCHER_WIDTH_PX }}
        items={others.map((provider) => ({
          id: provider.id,
          label: provider.displayName,
          icon: <ProviderIcon svg={provider.icon} />,
          onClick: () => onSwitch(provider.id),
        }))}
      />
    </>
  )
}
