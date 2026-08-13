'use client'

import * as React from 'react'
import type { Icon as PhosphorIcon } from '@phosphor-icons/react'
import { useMediaQuery } from '@/hooks/use-media-query'
import { cn } from '@/lib/utils'
import { Button, type ButtonProps } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Sheet, SheetDescription, SheetHeader, SheetPopup, SheetTitle } from '@/components/ui/sheet'

const SIDEBAR_COOKIE_NAME: string = 'sidebar_state'
const SIDEBAR_COOKIE_MAX_AGE: number = 60 * 60 * 24 * 7
const SIDEBAR_WIDTH: string = '16rem'
const SIDEBAR_WIDTH_MOBILE: string = '18rem'
const SIDEBAR_WIDTH_ICON: string = '3rem'
const SIDEBAR_KEYBOARD_SHORTCUT: string = 'b'

export type SidebarContextProps = {
  state: 'expanded' | 'collapsed'
  open: boolean
  setOpen: (open: boolean) => void
  openMobile: boolean
  setOpenMobile: (open: boolean) => void
  isMobile: boolean
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

/**
 * The sidebar context, or null outside a provider.
 *
 * For consumers that merely ADJUST themselves to the sidebar rather than drive
 * it — a pane squaring off the window edge the sidebar stopped covering, say —
 * and so must stay renderable on their own.
 */
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
  const isMobile = useMediaQuery('max-md')
  const [openMobile, setOpenMobile] = React.useState(false)

  // This is the internal state of the sidebar.
  // We use openProp and setOpenProp for control from outside the component.
  const [_open, _setOpen] = React.useState(defaultOpen)
  const open = openProp ?? _open
  const setOpen = React.useCallback(
    async (value: boolean | ((value: boolean) => boolean)) => {
      const openState = typeof value === 'function' ? value(open) : value
      if (setOpenProp) {
        setOpenProp(openState)
      } else {
        _setOpen(openState)
      }

      // This sets the cookie to keep the sidebar state.
      await cookieStore.set({
        expires: Date.now() + SIDEBAR_COOKIE_MAX_AGE * 1000,
        name: SIDEBAR_COOKIE_NAME,
        path: '/',
        value: String(openState),
      })
    },
    [setOpenProp, open],
  )

  // Helper to toggle the sidebar.
  const toggleSidebar = React.useCallback(() => {
    return isMobile ? setOpenMobile((open) => !open) : setOpen((open) => !open)
  }, [isMobile, setOpen])

  // Adds a keyboard shortcut to toggle the sidebar.
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

  // We add a state so that we can do data-state="expanded" or "collapsed".
  // This makes it easier to style the sidebar with Tailwind classes.
  const state = open ? 'expanded' : 'collapsed'

  const contextValue = React.useMemo<SidebarContextProps>(
    () => ({
      isMobile,
      open,
      openMobile,
      setOpen,
      setOpenMobile,
      state,
      toggleSidebar,
    }),
    [state, open, setOpen, isMobile, openMobile, toggleSidebar],
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

export function Sidebar({
  side = 'left',
  variant = 'sidebar',
  collapsible = 'offcanvas',
  className,
  children,
  ...props
}: React.ComponentProps<'div'> & {
  side?: 'left' | 'right'
  variant?: 'sidebar' | 'floating' | 'inset'
  collapsible?: 'offcanvas' | 'icon' | 'none'
}): React.ReactElement {
  const { isMobile, state, openMobile, setOpenMobile } = useSidebar()

  if (collapsible === 'none') {
    return (
      <div
        className={cn(
          'flex h-full w-(--sidebar-width) flex-col bg-sidebar text-sidebar-foreground',
          className,
        )}
        data-slot="sidebar"
        {...props}
      >
        {children}
      </div>
    )
  }

  if (isMobile) {
    return (
      <Sheet onOpenChange={setOpenMobile} open={openMobile} {...props}>
        <SheetPopup
          className="w-(--sidebar-width) bg-sidebar p-0 text-sidebar-foreground [&>button]:hidden"
          data-mobile="true"
          data-sidebar="sidebar"
          data-slot="sidebar"
          side={side}
          style={
            {
              '--sidebar-width': SIDEBAR_WIDTH_MOBILE,
            } as React.CSSProperties
          }
        >
          <SheetHeader className="sr-only">
            <SheetTitle>Sidebar</SheetTitle>
            <SheetDescription>Displays the mobile sidebar.</SheetDescription>
          </SheetHeader>
          <div className="flex h-full w-full flex-col">{children}</div>
        </SheetPopup>
      </Sheet>
    )
  }

  return (
    <div
      className="group peer hidden text-sidebar-foreground md:block"
      data-collapsible={state === 'collapsed' ? collapsible : ''}
      data-side={side}
      data-slot="sidebar"
      data-state={state}
      data-variant={variant}
    >
      {/* This is what handles the sidebar gap on desktop */}
      <div
        className={cn(
          'relative w-(--sidebar-width) bg-transparent transition-[width] duration-200 ease-linear',
          'group-data-[collapsible=offcanvas]:w-0',
          'group-data-[side=right]:rotate-180',
          variant === 'floating' || variant === 'inset'
            ? 'group-data-[collapsible=icon]:w-[calc(var(--sidebar-width-icon)+(--spacing(4)))]'
            : 'group-data-[collapsible=icon]:w-(--sidebar-width-icon)',
        )}
        data-slot="sidebar-gap"
      />
      <div
        className={cn(
          'fixed inset-y-0 z-10 hidden h-svh w-(--sidebar-width) transition-[left,right,width] duration-200 ease-linear md:flex',
          side === 'left'
            ? 'left-0 group-data-[collapsible=offcanvas]:left-[calc(var(--sidebar-width)*-1)]'
            : 'right-0 group-data-[collapsible=offcanvas]:right-[calc(var(--sidebar-width)*-1)]',
          // Adjust the padding for floating and inset variants.
          variant === 'floating' || variant === 'inset'
            ? 'p-2 group-data-[collapsible=icon]:w-[calc(var(--sidebar-width-icon)+(--spacing(4))+2px)]'
            : 'group-data-[collapsible=icon]:w-(--sidebar-width-icon) group-data-[side=left]:border-r group-data-[side=right]:border-l',
          className,
        )}
        data-slot="sidebar-container"
        {...props}
      >
        <div
          className="flex h-full w-full flex-col bg-sidebar group-data-[variant=floating]:rounded-lg group-data-[variant=floating]:border group-data-[variant=floating]:border-sidebar-border group-data-[variant=floating]:shadow-sm/5"
          data-sidebar="sidebar"
          data-slot="sidebar-inner"
        >
          {children}
        </div>
      </div>
    </div>
  )
}
export function SidebarHeader({
  className,
  ...props
}: React.ComponentProps<'div'>): React.ReactElement {
  return (
    <div
      // `pt-1` rather than the `p-2` this had on all four sides, because the
      // header's TOP is half of a gap the sidebar tab bar owns the other half of.
      // The bar below the context pill is `py-1.5`, so the rhythm down the top of
      // the sidebar is: pill wrapper `pb-1` (4px) + bar `pt-1.5` (6px) = 10px to
      // the switcher, and bar `pb-1.5` (6px) + this `pt-1` (4px) = 10px from the
      // switcher to whatever panel follows. It read 14px here and 6px in the
      // Chats panel, which drew the eye to the switcher sitting closer to one
      // neighbour than the other.
      //
      // It lives HERE, on the header every search panel is wrapped in, so a
      // third panel inherits the rhythm instead of picking its own padding.
      className={cn('flex flex-col gap-2 px-2 pt-1 pb-2 backdrop-blur-sm', className)}
      data-sidebar="header"
      data-slot="sidebar-header"
      {...props}
    />
  )
}

export function SidebarFooter({
  className,
  surface: _surface,
  ...props
}: React.ComponentProps<'div'> & {
  /** Crowbar compat: surface elevation flag (no visual effect) */
  surface?: boolean
}): React.ReactElement {
  return (
    <div
      className={cn('flex flex-col gap-2 p-2', className)}
      data-sidebar="footer"
      data-slot="sidebar-footer"
      {...props}
    />
  )
}
export const SidebarHeaderSearch = React.forwardRef<
  HTMLInputElement,
  Omit<React.ComponentProps<typeof Input>, 'onChange' | 'value' | 'size'> & {
    value: string
    onChange: (value: string) => void
    leftIcon: PhosphorIcon
  }
>(function SidebarHeaderSearch(
  { value, onChange, leftIcon: LeftIcon, placeholder = 'Search', className, ...props },
  ref,
) {
  return (
    <span className={cn('relative inline-flex min-w-0 flex-1', className)}>
      {LeftIcon && (
        <span className="pointer-events-none absolute inset-y-0 start-2 flex items-center text-muted-foreground/72">
          <LeftIcon className="size-3.5" />
        </span>
      )}
      <Input
        nativeInput
        ref={ref}
        value={value}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
        placeholder={placeholder}
        size="sm"
        unstyled
        className="h-6 min-w-0 w-full rounded-md px-2 ps-7 text-sm bg-transparent border-transparent outline-none"
        {...props}
      />
    </span>
  )
})

export const SidebarHeaderIconButton = React.forwardRef<
  HTMLButtonElement,
  Omit<ButtonProps, 'variant'>
>(function SidebarHeaderIconButton({ className, ...props }, ref) {
  return (
    <Button
      ref={ref}
      type="button"
      variant="ghost"
      compact
      className={cn('size-6 rounded-md p-0', className)}
      {...props}
    />
  )
})
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
