import { useRef, useState } from 'react'
import { CaretUpDown } from '@phosphor-icons/react'
import { Dropdown, dropdownTriggerClassName } from '@/components/ui/dropdown'
import { ProviderIcon } from '@/components/ui/provider-icon'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'

// Shared by trigger and menu. The menu takes it as an inline style because
// Dropdown's applyLockedWidth overwrites a class-set width but leaves style alone.
const SWITCHER_WIDTH_PX = 180
// Must equal SWITCHER_WIDTH_PX; Tailwind only generates literal class names.
const SWITCHER_WIDTH_CLASS = 'w-[180px]'

export interface ProviderSwitchDropdownProps {
  providers: AgentProvider[]
  currentProviderId: string
  onSwitch: (providerId: string) => void
  disabled?: boolean
}

export function ProviderSwitchDropdown({
  providers,
  currentProviderId,
  onSwitch,
  disabled = false,
}: ProviderSwitchDropdownProps) {
  const [isOpen, setIsOpen] = useState(false)
  const anchorRef = useRef<HTMLButtonElement>(null)

  // `current` includes disabled providers (disabling never touches running chats);
  // targets exclude them, or switching would spawn the CLI the user turned off.
  const current = providers.find((p) => p.id === currentProviderId)
  const others = providers.filter((p) => p.id !== currentProviderId && p.enabled)

  // Derived so the menu closes the same render the trigger goes disabled.
  const open = isOpen && !disabled

  // A spread because Dropdown's `items` and `children` are mutually exclusive.
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
      {/* -mr-[9px] (8px padding + 1px border) puts the ghost trigger's caret on the
          terminal's last column; justify-between because buttonVariants centres. */}
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
          {/* Not a menuitem: there is nothing here to select. */}
          <p className="ui-font ui-text-xs px-2 py-1.5 text-muted-foreground">
            No other provider is enabled. Turn one on in Settings to switch mid-chat — this
            conversation&apos;s context comes with you.
          </p>
        </Dropdown>
      ) : null}
    </>
  )
}
