import { Tooltip as TooltipPrimitive } from '@base-ui/react/tooltip'
import type React from 'react'
import Keybinding from '@/components/ui/keybinding'
import { cn } from '@/utils/cn'

type Side = 'top' | 'bottom' | 'left' | 'right'

const tooltipContentBase =
  'ui-text-sm pointer-events-none whitespace-nowrap rounded-lg border border-border/70 bg-card/95 px-2.5 py-1.5 text-foreground shadow-lg backdrop-blur-sm animate-in fade-in-0 zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95 data-[side=bottom]:slide-in-from-top-1 data-[side=left]:slide-in-from-right-1 data-[side=right]:slide-in-from-left-1 data-[side=top]:slide-in-from-bottom-1'

export const tooltipContentPrimary =
  'w-fit text-balance rounded-md bg-primary px-3 py-1.5 text-primary-foreground text-xs'

const TOOLTIP_DELAY_MS = 150

export function TooltipProvider({ children }: { children: React.ReactNode }) {
  return (
    <TooltipPrimitive.Provider delay={TOOLTIP_DELAY_MS} timeout={0}>
      {children}
    </TooltipPrimitive.Provider>
  )
}

export function WithTooltip({
  trigger,
  content,
  side = 'top',
  sideOffset = 6,
  delay = TOOLTIP_DELAY_MS,
  className,
  popupClassName,
}: {
  trigger: React.ReactElement
  content: React.ReactNode
  side?: Side
  sideOffset?: number
  delay?: number
  className?: string
  popupClassName?: string
}) {
  return (
    <TooltipPrimitive.Root disableHoverablePopup>
      <TooltipPrimitive.Trigger delay={delay} render={trigger} />
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Positioner
          side={side}
          sideOffset={sideOffset}
          collisionPadding={8}
          className="z-[99999]"
        >
          <TooltipPrimitive.Popup
            className={cn(popupClassName ?? tooltipContentBase, className)}
            data-slot="tooltip-content"
          >
            {content}
          </TooltipPrimitive.Popup>
        </TooltipPrimitive.Positioner>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  )
}

interface TooltipProps {
  content: string
  children: React.ReactNode
  side?: Side
  shortcut?: string
  className?: string
  triggerClassName?: string
}

export default function TooltipCompound({
  content,
  children,
  side = 'top',
  shortcut,
  className,
  triggerClassName,
}: TooltipProps) {
  return (
    <WithTooltip
      trigger={<span className={cn('inline-flex items-center', triggerClassName)}>{children}</span>}
      content={
        <>
          {content}
          {shortcut && <Keybinding binding={shortcut} />}
        </>
      }
      side={side}
      className={cn(shortcut && 'flex items-center gap-2', className)}
    />
  )
}
