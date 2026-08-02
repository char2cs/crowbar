import { Toast } from '@base-ui/react/toast'
import {
  CircleAlertIcon,
  CircleCheckIcon,
  InfoIcon,
  LoaderCircleIcon,
  TriangleAlertIcon,
} from 'lucide-react'
import type React from 'react'
import { cn } from '@/lib/utils'
import { buttonVariants } from '@/components/ui/button-variants'

const TOAST_ICONS = {
  error: CircleAlertIcon,
  info: InfoIcon,
  loading: LoaderCircleIcon,
  success: CircleCheckIcon,
  warning: TriangleAlertIcon,
} as const

type ToastData = {
  rootProps?: Omit<
    React.ComponentProps<typeof Toast.Root>,
    'children' | 'className' | 'swipeDirection' | 'toast'
  >
  tooltipStyle?: boolean
}

function upsertReplayClassName(toast: { type?: string; updateKey?: number }): string | undefined {
  const k = toast.updateKey ?? 0
  if (k <= 0) return undefined
  const isEven = k % 2 === 0
  if (toast.type === 'error') {
    return isEven ? 'animate-toast-error-even' : 'animate-toast-error-odd'
  }
  return isEven ? 'animate-toast-success-even' : 'animate-toast-success-odd'
}

function AnchoredToasts({
  portalProps,
}: {
  portalProps?: React.ComponentProps<typeof Toast.Portal>
}): React.ReactElement {
  const { toasts } = Toast.useToastManager()

  return (
    <Toast.Portal data-slot="toast-portal-anchored" {...portalProps}>
      <Toast.Viewport className="outline-none" data-slot="toast-viewport-anchored">
        {toasts.map((toast) => {
          const Icon = toast.type ? TOAST_ICONS[toast.type as keyof typeof TOAST_ICONS] : null
          const toastData = toast.data as ToastData | undefined
          const tooltipStyle = toastData?.tooltipStyle ?? false
          const positionerProps = toast.positionerProps

          if (!positionerProps?.anchor) {
            return null
          }

          return (
            <Toast.Positioner
              key={toast.id}
              className="z-50 max-w-[min(--spacing(64),var(--available-width))]"
              data-slot="toast-positioner"
              sideOffset={positionerProps.sideOffset ?? 4}
              toast={toast}
            >
              <Toast.Root
                className={cn(
                  'relative text-balance border bg-popover not-dark:bg-clip-padding text-popover-foreground text-xs transition-[scale,opacity] before:pointer-events-none before:absolute before:inset-0 before:shadow-[0_1px_--theme(--color-black/4%)] data-ending-style:scale-98 data-starting-style:scale-98 data-ending-style:opacity-0 data-starting-style:opacity-0 dark:before:shadow-[0_-1px_--theme(--color-white/6%)]',
                  tooltipStyle
                    ? 'rounded-md shadow-md/5 before:rounded-[calc(var(--radius-md)-1px)]'
                    : 'rounded-lg shadow-lg/5 before:rounded-[calc(var(--radius-lg)-1px)]',
                  upsertReplayClassName(toast),
                )}
                {...toastData?.rootProps}
                data-oracle-id="toast-popup"
                data-slot="toast-popup"
                toast={toast}
              >
                {tooltipStyle ? (
                  <Toast.Content className="pointer-events-auto px-2 py-1">
                    <Toast.Title data-oracle-id="toast-title" data-slot="toast-title" />
                  </Toast.Content>
                ) : (
                  <Toast.Content className="pointer-events-auto flex flex-col gap-2 overflow-hidden px-3.5 py-3 text-sm">
                    <div className="flex gap-2">
                      {Icon && (
                        <div
                          className="[&>svg]:h-lh [&>svg]:w-4 [&_svg]:pointer-events-none [&_svg]:shrink-0"
                          data-oracle-id="toast-icon"
                          data-slot="toast-icon"
                          // Same reduced-motion spinner exemption as the stacked
                          // variant above — see index.css.
                          data-essential-motion=""
                        >
                          <Icon className="in-data-[type=loading]:animate-spin in-data-[type=error]:text-destructive in-data-[type=info]:text-info in-data-[type=success]:text-success in-data-[type=warning]:text-warning in-data-[type=loading]:opacity-80" />
                        </div>
                      )}

                      <div className="flex flex-col gap-0.5">
                        <Toast.Title
                          className="min-w-0 break-words font-medium"
                          data-oracle-id="toast-title"
                          data-slot="toast-title"
                        />
                        <Toast.Description
                          className="min-w-0 break-words text-muted-foreground"
                          data-oracle-id="toast-description"
                          data-slot="toast-description"
                        />
                      </div>
                    </div>
                    {toast.actionProps && (
                      <div className="flex justify-end" data-slot="toast-action-row">
                        <Toast.Action
                          className={buttonVariants({ size: 'xs' })}
                          data-slot="toast-action"
                          onClick={toast.actionProps.onClick}
                        >
                          {toast.actionProps.children}
                        </Toast.Action>
                      </div>
                    )}
                  </Toast.Content>
                )}
              </Toast.Root>
            </Toast.Positioner>
          )
        })}
      </Toast.Viewport>
    </Toast.Portal>
  )
}

export const toastManager: ReturnType<typeof Toast.createToastManager> = Toast.createToastManager()

export const anchoredToastManager: ReturnType<typeof Toast.createToastManager> =
  Toast.createToastManager()

export interface AnchoredToastProviderProps extends Toast.Provider.Props {
  portalProps?: React.ComponentProps<typeof Toast.Portal>
}

export function AnchoredToastProvider({
  children,
  portalProps,
  ...props
}: AnchoredToastProviderProps): React.ReactElement {
  return (
    <Toast.Provider toastManager={anchoredToastManager} {...props}>
      {children}
      <AnchoredToasts portalProps={portalProps} />
    </Toast.Provider>
  )
}
