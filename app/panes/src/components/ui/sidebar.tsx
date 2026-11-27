'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'

const SIDEBAR_WIDTH: string = '16rem'
const SIDEBAR_WIDTH_ICON: string = '3rem'
const SIDEBAR_KEYBOARD_SHORTCUT: string = 'b'

export type SidebarContextProps = {
  state: 'expanded' | 'collapsed'
  open: boolean
  setOpen: (open: boolean) => void
  toggleSidebar: () => void
}

const SidebarContext: React.Context<SidebarContextProps | null> =
  React.createContext<SidebarContextProps | null>(null)

export function useSidebar(): SidebarContextProps {
  const context = React.useContext(SidebarContext)
  if (!context) {
    throw new Error('useSidebar must be used within a SidebarProvider.')
  }

  return context
}

export function useSidebarOptional(): SidebarContextProps | null {
  return React.useContext(SidebarContext)
}

export function SidebarProvider({
  defaultOpen = true,
  open: openProp,
  onOpenChange: setOpenProp,
  className,
  style,
  children,
  ...props
}: React.ComponentProps<'div'> & {
  defaultOpen?: boolean
  open?: boolean
  onOpenChange?: (open: boolean) => void
}): React.ReactElement {
  const [_open, _setOpen] = React.useState(defaultOpen)
  const open = openProp ?? _open
  const setOpen = React.useCallback(
    (value: boolean | ((value: boolean) => boolean)) => {
      const openState = typeof value === 'function' ? value(open) : value
      if (setOpenProp) {
        setOpenProp(openState)
      } else {
        _setOpen(openState)
      }
    },
    [setOpenProp, open],
  )

  const toggleSidebar = React.useCallback(() => setOpen((open) => !open), [setOpen])

  React.useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent): void => {
      if (event.key === SIDEBAR_KEYBOARD_SHORTCUT && (event.metaKey || event.ctrlKey)) {
        event.preventDefault()
        toggleSidebar()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [toggleSidebar])

  const state = open ? 'expanded' : 'collapsed'

  const contextValue = React.useMemo<SidebarContextProps>(
    () => ({
      open,
      setOpen,
      state,
      toggleSidebar,
    }),
    [state, open, setOpen, toggleSidebar],
  )

  return (
    <SidebarContext.Provider value={contextValue}>
      <div
        className={cn(
          'group/sidebar-wrapper flex min-h-svh w-full has-data-[variant=inset]:bg-sidebar',
          className,
        )}
        data-slot="sidebar-wrapper"
        style={
          {
            '--sidebar-width': SIDEBAR_WIDTH,
            '--sidebar-width-icon': SIDEBAR_WIDTH_ICON,
            ...style,
          } as React.CSSProperties
        }
        {...props}
      >
        {children}
      </div>
    </SidebarContext.Provider>
  )
}

export function SidebarHeader({
  className,
  ...props
}: React.ComponentProps<'div'>): React.ReactElement {
  return (
    <div
      className={cn('flex flex-col gap-2 pl-3 pr-3 backdrop-blur-sm', className)}
      data-sidebar="header"
      data-slot="sidebar-header"
      {...props}
    />
  )
}

export function SidebarEmptyActionState({
  icon,
  message,
  description,
  actionLabel,
  onAction,
  actionDisabled = false,
  className,
  actionClassName,
  tone = 'neutral',
  children,
  ...props
}: React.ComponentProps<'div'> & {
  icon?: React.ReactNode
  message: React.ReactNode
  description?: React.ReactNode
  actionLabel?: React.ReactNode
  onAction?: () => void
  actionDisabled?: boolean
  actionClassName?: string
  tone?: 'neutral' | 'error' | 'success'
}): React.ReactElement {
  return (
    <div
      className={cn(
        'ui-font flex min-h-24 select-none flex-col items-center justify-center gap-1.5 px-3 py-6 text-center text-muted-foreground',
        className,
      )}
      {...props}
    >
      {icon ? (
        <span
          className={cn(
            'mb-0.5 flex size-7 items-center justify-center text-muted-foreground',
            tone === 'error' && 'text-destructive',
            tone === 'success' && 'text-success',
          )}
        >
          {icon}
        </span>
      ) : null}
      <div
        className={cn(
          'ui-text-sm leading-[1.35]',
          tone === 'error' && 'text-destructive',
          tone === 'success' && 'text-success',
        )}
      >
        {message}
      </div>
      {description ? (
        <div className="ui-text-xs max-w-[24ch] leading-[1.35] text-muted-foreground">
          {description}
        </div>
      ) : null}
      {actionLabel && onAction ? (
        <Button
          type="button"
          variant="ghost"
          compact
          className={cn(
            'ui-text-xs h-6 px-2 text-muted-foreground hover:text-foreground',
            actionClassName,
          )}
          disabled={actionDisabled}
          onClick={onAction}
        >
          {actionLabel}
        </Button>
      ) : null}
      {children}
    </div>
  )
}
