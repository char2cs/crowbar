import { ArrowCounterClockwise as RotateCcw } from '@phosphor-icons/react'
import React from 'react'
import { settingRowMatchesQuery } from '@/features/settings/lib/settings-row-search'
import { useSettingsStore } from '@/features/settings/store'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { cn } from '@/utils/cn'

/**
 * Lets SettingRows report whether they match the active settings search, so a
 * Section can hide its heading once every row inside it is filtered out.
 * `visible: null` unregisters a row (unmount).
 */
const SectionRowVisibilityContext = React.createContext<
  ((rowId: string, visible: boolean | null) => void) | null
>(null)

interface SectionProps {
  title: string
  description?: string
  children: React.ReactNode
  className?: string
}

export default function Section({ title, description, children, className }: SectionProps) {
  const isSearchActive = useSettingsStore((state) => state.search.query.trim().length > 0)
  const rowVisibilityRef = React.useRef(new Map<string, boolean>())
  const [visibleRowCount, setVisibleRowCount] = React.useState(0)

  const reportRowVisibility = React.useCallback((rowId: string, visible: boolean | null) => {
    if (visible === null) {
      rowVisibilityRef.current.delete(rowId)
    } else {
      rowVisibilityRef.current.set(rowId, visible)
    }
    let count = 0
    rowVisibilityRef.current.forEach((isVisible) => {
      if (isVisible) count += 1
    })
    setVisibleRowCount(count)
  }, [])

  // While searching, hide the whole section (heading included) once every row
  // inside it has been filtered out. Rows stay mounted (they render null), so
  // the `hidden` attribute is used instead of unmounting the subtree.
  const isHiddenBySearch = isSearchActive && visibleRowCount === 0

  return (
    <SectionRowVisibilityContext.Provider value={reportRowVisibility}>
      <section
        hidden={isHiddenBySearch}
        className={cn('px-1 py-0.5 first:[&>.settings-section-header]:hidden', className)}
        data-settings-section={title}
      >
        <div className="settings-section-header mb-2 px-1 py-1.5">
          <Label className="ui-font ui-text-base font-medium text-foreground">{title}</Label>
          {description && <p className="ui-font ui-text-sm text-muted-foreground">{description}</p>}
        </div>
        <div className="space-y-2">{children}</div>
      </section>
    </SectionRowVisibilityContext.Provider>
  )
}

interface SettingRowProps {
  label: string
  labelAccessory?: React.ReactNode
  description?: React.ReactNode
  children: React.ReactNode
  className?: string
  onReset?: () => void
  canReset?: boolean
  resetLabel?: string
}

export function SettingRow({
  label,
  labelAccessory,
  description,
  children,
  className,
  onReset,
  canReset = !!onReset,
  resetLabel,
}: SettingRowProps) {
  const controlRef = React.useRef<HTMLDivElement>(null)
  const rowId = React.useId()
  const labelId = `${rowId}-label`
  const descriptionId = `${rowId}-description`

  const searchQuery = useSettingsStore((state) => state.search.query)
  const matchesSearch = settingRowMatchesQuery(searchQuery, label, description)
  const reportRowVisibility = React.useContext(SectionRowVisibilityContext)

  React.useEffect(() => {
    if (!reportRowVisibility) return
    reportRowVisibility(rowId, matchesSearch)
    return () => reportRowVisibility(rowId, null)
  }, [reportRowVisibility, rowId, matchesSearch])

  const interactiveSelector =
    "button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [role='button'], [role='switch'], [tabindex]:not([tabindex='-1'])"
  const passthroughSelector =
    "button, input, select, textarea, a, label, [role='button'], [role='switch'], [data-slot='button'], [data-setting-interactive-root='true']"

  const getPrimaryInteractive = React.useCallback(() => {
    const controlRoot = controlRef.current
    if (!controlRoot) return null

    const primaryInteractive =
      controlRoot.querySelector<HTMLElement>(
        "[data-setting-primary-control='true'], [data-setting-interactive-root='true']",
      ) ?? controlRoot.querySelector<HTMLElement>(interactiveSelector)

    if (!primaryInteractive) return null

    return primaryInteractive.matches(interactiveSelector)
      ? primaryInteractive
      : primaryInteractive.querySelector<HTMLElement>(interactiveSelector)
  }, [interactiveSelector])

  React.useLayoutEffect(() => {
    const control = getPrimaryInteractive()
    if (!control) return

    if (!control.getAttribute('aria-labelledby') && !control.getAttribute('aria-label')) {
      control.setAttribute('aria-labelledby', labelId)
    }

    if (description && !control.getAttribute('aria-describedby')) {
      control.setAttribute('aria-describedby', descriptionId)
    }
  }, [description, descriptionId, getPrimaryInteractive, labelId])

  // Mouse-only convenience delegation: clicking anywhere on the row forwards
  // activation to the row's primary control, unless the click already landed
  // on a real interactive descendant (passthroughSelector) — those handle
  // their own clicks natively and this would double-fire. There is
  // deliberately NO keyboard counterpart and no row tabIndex: the primary
  // control is itself natively tabbable (button/input/select/switch) and the
  // useLayoutEffect above wires the row's label/description to it via
  // aria-labelledby/aria-describedby, so keyboard users Tab straight to the
  // labeled control — a row-level tab stop would only double every stop in
  // Settings.
  const handleRowClick = (event: React.MouseEvent<HTMLDivElement>) => {
    const target = event.target as HTMLElement

    if (target.closest(passthroughSelector)) {
      return
    }

    const segmentedControl = controlRef.current?.querySelector<HTMLElement>(
      "[data-setting-segmented-control='true']",
    )
    if (segmentedControl) {
      const segmentedItems = Array.from(
        segmentedControl.querySelectorAll<HTMLElement>("[role='button']"),
      ).filter((item) => !item.hasAttribute('disabled'))
      const activeIndex = segmentedItems.findIndex(
        (item) => item.getAttribute('data-setting-segmented-active') === 'true',
      )

      if (segmentedItems.length > 0) {
        const nextIndex = activeIndex >= 0 ? (activeIndex + 1) % segmentedItems.length : 0
        const nextItem = segmentedItems[nextIndex]
        nextItem?.focus()
        nextItem?.click()
        return
      }
    }

    const firstInteractive = getPrimaryInteractive()
    if (!firstInteractive) return

    if (firstInteractive.getAttribute('role') === 'combobox') {
      firstInteractive.focus()
      firstInteractive.click()
      return
    }

    if (firstInteractive.getAttribute('aria-expanded') != null) {
      firstInteractive.focus()
      firstInteractive.click()
      return
    }

    if (
      firstInteractive instanceof HTMLInputElement &&
      firstInteractive.type !== 'checkbox' &&
      firstInteractive.type !== 'radio'
    ) {
      firstInteractive.focus()
      firstInteractive.select?.()
      return
    }

    firstInteractive.focus()
    firstInteractive.click()
  }

  // Row-level search filtering: while a settings search query is active, only
  // rows whose label/description match it render. Placed after all hooks.
  if (!matchesSearch) return null

  return (
    <div
      // Presentation, not group: this row div is a mouse-only click-to-focus
      // hit area around its real control (see handleRowClick) — the same
      // delegation pattern as InputGroupAddon. The accessible name/description
      // live on the control itself (aria-labelledby/aria-describedby wired in
      // the useLayoutEffect above), so this wrapper carries no semantics of
      // its own and needs no aria attributes or keyboard handler.
      role="presentation"
      className={cn(
        'flex items-center justify-between gap-3 rounded-lg px-1 py-2 select-none transition-colors hover:bg-muted/50 focus-within:bg-muted/50 max-[640px]:flex-col max-[640px]:items-stretch max-[640px]:gap-2',
        className,
      )}
      onClick={handleRowClick}
    >
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <Label
            id={labelId}
            className="ui-font ui-text-sm cursor-default font-normal text-foreground"
          >
            {label}
          </Label>
          {labelAccessory}
          {onReset ? (
            <span className="flex size-5 items-center justify-center">
              <Button
                type="button"
                variant="ghost"
                onClick={onReset}
                disabled={!canReset}
                aria-label={resetLabel || `Reset ${label}`}
                tooltip={canReset ? resetLabel || `Reset ${label}` : undefined}
                className={cn(!canReset && 'pointer-events-none invisible')}
                compact
              >
                <RotateCcw />
              </Button>
            </span>
          ) : null}
        </div>
        {description && (
          <div
            id={descriptionId}
            className="ui-font ui-text-sm cursor-default text-muted-foreground"
          >
            {description}
          </div>
        )}
      </div>
      <div
        ref={controlRef}
        className="ui-font ui-text-sm shrink-0 select-auto [--app-ui-badge-height:1.5rem] [--app-ui-button-compact-height:1.5rem] [--app-ui-button-compact-min-width:1.5rem] [--app-ui-button-height:1.5rem] [--app-ui-button-min-width:1.5rem] [--app-ui-control-font-size:var(--ui-text-sm)] max-[640px]:w-full max-[640px]:shrink max-[640px]:[&>input]:w-full max-[640px]:[&>textarea]:w-full"
      >
        {children}
      </div>
    </div>
  )
}
