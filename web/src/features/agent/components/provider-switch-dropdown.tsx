import { useRef, useState } from 'react'
import { CaretUpDown } from '@phosphor-icons/react'
import { Dropdown, DROPDOWN_TRIGGER_BASE } from '@/components/ui/dropdown'
import { cn } from '@/lib/utils'
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
      <button
        ref={anchorRef}
        type="button"
        onClick={() => setIsOpen((open) => !open)}
        className={cn(DROPDOWN_TRIGGER_BASE, 'h-7 text-foreground')}
      >
        {current && <ProviderIcon svg={current.icon} />}
        <span className="truncate">{current?.displayName ?? 'Provider'}</span>
        <CaretUpDown size={12} className="text-muted-foreground" />
      </button>
      <Dropdown
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        anchorRef={anchorRef}
        anchorSide="top"
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
