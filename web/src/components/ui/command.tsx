'use client'

import { Dialog as CommandDialogPrimitive } from '@base-ui/react/dialog'
import { SearchIcon } from 'lucide-react'
import type * as React from 'react'
import { cn } from '@/lib/utils'
import {
  Autocomplete,
  AutocompleteCollection,
  AutocompleteEmpty,
  AutocompleteGroup,
  AutocompleteGroupLabel,
  AutocompleteInput,
  AutocompleteItem,
  AutocompleteList,
  AutocompleteSeparator,
} from '@/components/ui/autocomplete'

export const CommandDialog: typeof CommandDialogPrimitive.Root = CommandDialogPrimitive.Root

export const CommandDialogPortal: typeof CommandDialogPrimitive.Portal =
  CommandDialogPrimitive.Portal

export const CommandCreateHandle: typeof CommandDialogPrimitive.createHandle =
  CommandDialogPrimitive.createHandle

export function CommandDialogTrigger(
  props: CommandDialogPrimitive.Trigger.Props,
): React.ReactElement {
  return <CommandDialogPrimitive.Trigger data-slot="command-dialog-trigger" {...props} />
}

export function CommandDialogBackdrop({
  className,
  ...props
}: CommandDialogPrimitive.Backdrop.Props): React.ReactElement {
  return (
    <CommandDialogPrimitive.Backdrop
      className={cn(
        'fixed inset-0 z-50 bg-black/32 backdrop-blur-sm transition-all duration-200 data-ending-style:opacity-0 data-starting-style:opacity-0',
        className,
      )}
      data-slot="command-dialog-backdrop"
      {...props}
    />
  )
}

export function CommandDialogViewport({
  className,
  ...props
}: CommandDialogPrimitive.Viewport.Props): React.ReactElement {
  return (
    <CommandDialogPrimitive.Viewport
      className={cn(
        'fixed inset-0 z-50 flex flex-col items-center px-4 py-[max(--spacing(4),4vh)] sm:py-[10vh]',
        className,
      )}
      data-slot="command-dialog-viewport"
      {...props}
    />
  )
}

export function CommandDialogPopup({
  className,
  children,
  portalProps,
  ...props
}: CommandDialogPrimitive.Popup.Props & {
  portalProps?: CommandDialogPrimitive.Portal.Props
}): React.ReactElement {
  return (
    <CommandDialogPortal {...portalProps}>
      <CommandDialogBackdrop />
      <CommandDialogViewport>
        <CommandDialogPrimitive.Popup
          className={cn(
            'relative row-start-2 flex max-h-105 min-h-0 w-full min-w-0 max-w-xl -translate-y-[calc(1.25rem*var(--nested-dialogs))] scale-[calc(1-0.1*var(--nested-dialogs))] flex-col rounded-2xl border bg-popover not-dark:bg-clip-padding text-popover-foreground opacity-[calc(1-0.1*var(--nested-dialogs))] shadow-lg/5 outline-none transition-[scale,opacity,translate] duration-200 ease-in-out will-change-transform before:pointer-events-none before:absolute before:inset-0 before:rounded-[calc(var(--radius-2xl)-1px)] before:bg-muted/72 before:shadow-[0_1px_--theme(--color-black/4%)] data-nested:data-ending-style:translate-y-8 data-nested:data-starting-style:translate-y-8 data-nested-dialog-open:origin-top data-ending-style:scale-98 data-starting-style:scale-98 data-ending-style:opacity-0 data-starting-style:opacity-0 **:data-[slot=scroll-area-viewport]:data-has-overflow-y:pe-1 dark:before:shadow-[0_-1px_--theme(--color-white/6%)]',
            className,
          )}
          data-slot="command-dialog-popup"
          {...props}
        >
          {children}
        </CommandDialogPrimitive.Popup>
      </CommandDialogViewport>
    </CommandDialogPortal>
  )
}

export function Command({
  autoHighlight = 'always',
  keepHighlight = true,
  isVisible: _isVisible,
  onClose: _onClose,
  placement: _placement,
  title: _title,
  className,
  ...props
}: React.ComponentProps<typeof Autocomplete> & {
  /** Crowbar compat: visibility flag (ignored; Command is always open inline) */
  isVisible?: boolean
  /** Crowbar compat: close handler (ignored) */
  onClose?: () => void
  /** Crowbar compat: placement hint (ignored) */
  placement?: 'top' | 'bottom'
  /** Crowbar compat: title label (ignored) */
  title?: string
  className?: string
}): React.ReactElement {
  return (
    <div className={className} data-slot="command">
      <Autocomplete
        autoHighlight={autoHighlight}
        inline
        keepHighlight={keepHighlight}
        open
        {...props}
      />
    </div>
  )
}

export function CommandInput({
  className,
  placeholder = undefined,
  onChange,
  ...props
}: Omit<React.ComponentProps<typeof AutocompleteInput>, 'onChange'> & {
  /** Accept both the Base UI event handler and the legacy Crowbar string handler */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  onChange?: ((event: any) => void) | ((value: string) => void)
}): React.ReactElement {
  return (
    <div className="px-2.5 py-1.5">
      <AutocompleteInput
        autoFocus
        className={cn(
          'border-transparent! bg-transparent! shadow-none before:hidden has-focus-visible:ring-0',
          className,
        )}
        placeholder={placeholder}
        size="lg"
        startAddon={<SearchIcon />}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        onChange={onChange as any}
        {...props}
      />
    </div>
  )
}

export function CommandList({
  className,
  ...props
}: React.ComponentProps<typeof AutocompleteList>): React.ReactElement {
  return (
    <AutocompleteList
      className={cn('not-empty:scroll-py-2 not-empty:p-2', className)}
      data-slot="command-list"
      {...props}
    />
  )
}

export function CommandEmpty({
  className,
  ...props
}: React.ComponentProps<typeof AutocompleteEmpty>): React.ReactElement {
  return (
    <AutocompleteEmpty
      className={cn('not-empty:py-6', className)}
      data-slot="command-empty"
      {...props}
    />
  )
}

export function CommandPanel({
  className,
  ...props
}: React.ComponentProps<'div'>): React.ReactElement {
  return (
    <div
      className={cn(
        'relative -mx-px not-has-[+[data-slot=command-footer]]:-mb-px min-h-0 rounded-t-xl not-has-[+[data-slot=command-footer]]:rounded-b-2xl border border-b-0 bg-popover bg-clip-padding shadow-xs/5 [clip-path:inset(0_1px)] not-has-[+[data-slot=command-footer]]:[clip-path:inset(0_1px_1px_1px_round_0_0_calc(var(--radius-2xl)-1px)_calc(var(--radius-2xl)-1px))] before:pointer-events-none before:absolute before:inset-0 before:rounded-t-[calc(var(--radius-xl)-1px)] **:data-[slot=scroll-area-scrollbar]:mt-2',
        className,
      )}
      {...props}
    />
  )
}

export function CommandGroup({
  className,
  ...props
}: React.ComponentProps<typeof AutocompleteGroup>): React.ReactElement {
  return <AutocompleteGroup className={className} data-slot="command-group" {...props} />
}

export function CommandGroupLabel({
  className,
  ...props
}: React.ComponentProps<typeof AutocompleteGroupLabel>): React.ReactElement {
  return <AutocompleteGroupLabel className={className} data-slot="command-group-label" {...props} />
}

export function CommandCollection({
  ...props
}: React.ComponentProps<typeof AutocompleteCollection>): React.ReactElement {
  return <AutocompleteCollection data-slot="command-collection" {...props} />
}

export function CommandItem({
  className,
  isSelected: _isSelected,
  type: _type,
  onClick,
  ...props
}: React.ComponentProps<typeof AutocompleteItem> & {
  /** Crowbar compat: visual selection state (ignored; Base UI uses data-highlighted) */
  isSelected?: boolean
  /** Crowbar compat: html button type attribute (ignored) */
  type?: string
  /** Crowbar compat: click handler */
  onClick?: React.MouseEventHandler<HTMLDivElement>
}): React.ReactElement {
  return (
    <AutocompleteItem
      className={cn('py-1.5', className)}
      data-slot="command-item"
      onClick={onClick as React.ComponentProps<typeof AutocompleteItem>['onClick']}
      {...props}
    />
  )
}

/** Crowbar compat: header section within a command surface */
export function CommandHeader({
  children,
  className,
  onClose: _onClose,
  showClearButton: _showClearButton,
}: {
  children?: React.ReactNode
  className?: string
  onClose?: () => void
  showClearButton?: boolean
}): React.ReactElement {
  return (
    <div className={className} data-slot="command-header">
      {children}
    </div>
  )
}

export function CommandSeparator({
  className,
  ...props
}: React.ComponentProps<typeof AutocompleteSeparator>): React.ReactElement {
  return (
    <AutocompleteSeparator
      className={cn('my-2', className)}
      data-slot="command-separator"
      {...props}
    />
  )
}

export function CommandShortcut({
  className,
  ...props
}: React.ComponentProps<'kbd'>): React.ReactElement {
  return (
    <kbd
      className={cn(
        'ms-auto font-medium font-sans text-muted-foreground/72 text-xs tracking-widest',
        className,
      )}
      data-slot="command-shortcut"
      {...props}
    />
  )
}

export function CommandFooter({
  className,
  ...props
}: React.ComponentProps<'div'>): React.ReactElement {
  return (
    <div
      className={cn(
        'flex items-center justify-between gap-2 rounded-b-[calc(var(--radius-2xl)-1px)] border-t px-5 py-3 text-muted-foreground text-xs',
        className,
      )}
      data-slot="command-footer"
      {...props}
    />
  )
}

export { CommandDialogPrimitive }
