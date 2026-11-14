import {
  AlignBottomSimpleIcon,
  AlignLeftSimpleIcon,
  AlignRightSimpleIcon,
  AlignTopSimpleIcon,
  SquareHalfIcon,
  StackIcon,
} from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { nextDock, type ConsoleDock } from '@/features/console/lib/dock-prefs'
import { useConsoleStore } from '@/features/console/stores/console-store'
import { IS_MAC } from '@/utils/platform'
import { cn } from '@/utils/cn'

const DOCK_ICON = {
  top: AlignTopSimpleIcon,
  right: AlignRightSimpleIcon,
  bottom: AlignBottomSimpleIcon,
  left: AlignLeftSimpleIcon,
} satisfies Record<ConsoleDock, unknown>

const label = (dock: ConsoleDock) => dock[0].toUpperCase() + dock.slice(1)

/** The console's own strip: dock position and overlay/push, icon-only. */
export function ConsoleHeader() {
  const dock = useConsoleStore((s) => s.dock)
  const mode = useConsoleStore((s) => s.mode)
  const DockIcon = DOCK_ICON[dock]
  const ModeIcon = mode === 'overlay' ? StackIcon : SquareHalfIcon

  return (
    <header
      data-tauri-drag-region
      className={cn(
        'border-border flex h-9 shrink-0 items-center justify-end gap-1 border-b px-2',
        // The window controls sit at the top-left corner this strip can reach.
        IS_MAC && (dock === 'top' || dock === 'left') && 'pl-20',
      )}
    >
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={`Dock position: ${label(dock)}`}
        tooltip={`Dock position: ${label(dock)}. Click to move it to the ${label(nextDock(dock))}.`}
        tooltipSide="bottom"
        onClick={() => useConsoleStore.getState().cycleDock()}
      >
        <DockIcon />
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={mode === 'overlay' ? 'Mode: Overlay' : 'Mode: Push'}
        aria-pressed={mode === 'push'}
        tooltip={
          mode === 'overlay'
            ? 'Overlay: floating over the app. Click to make it take space beside the app.'
            : 'Push: taking space, the app shrinks to fit. Click to float it over the app.'
        }
        tooltipSide="bottom"
        onClick={() => useConsoleStore.getState().toggleMode()}
      >
        <ModeIcon />
      </Button>
    </header>
  )
}
