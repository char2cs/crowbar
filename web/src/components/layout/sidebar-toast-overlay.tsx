import { Toast } from '@base-ui/react/toast'
import {
  CircleAlertIcon,
  CircleCheckIcon,
  InfoIcon,
  LoaderCircleIcon,
  TriangleAlertIcon,
  X,
} from 'lucide-react'
import { buttonVariants } from '@/components/ui/button'
import { toastManager } from '@/lib/toast-manager'

const TOAST_ICONS = {
  error: CircleAlertIcon,
  info: InfoIcon,
  loading: LoaderCircleIcon,
  success: CircleCheckIcon,
  warning: TriangleAlertIcon,
} as const

interface SidebarToastOverlayProps {
  sidebarOpen: boolean
  sidebarSide: 'left' | 'right'
}

function SidebarToastItem({
  toast,
  swipeDirection,
}: {
  toast: ReturnType<typeof Toast.useToastManager>['toasts'][number]
  swipeDirection: ('left' | 'right' | 'down')[]
}) {
  const Icon = toast.type ? TOAST_ICONS[toast.type as keyof typeof TOAST_ICONS] : null

  return (
    <Toast.Root
      toast={toast}
      swipeDirection={swipeDirection}
      className="pointer-events-auto relative flex w-full items-center justify-between gap-1.5 overflow-hidden rounded-lg border bg-popover px-3.5 py-3 text-sm text-popover-foreground shadow-lg/5 not-dark:bg-clip-padding transition-opacity duration-200 data-starting-style:opacity-0 data-ending-style:opacity-0"
    >
      <Toast.Content className="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden">
        <div className="flex min-w-0 flex-1 gap-2">
          {Icon && (
            <div
              className="[&>svg]:h-lh [&>svg]:w-4 [&_svg]:pointer-events-none [&_svg]:shrink-0"
              data-slot="toast-icon"
            >
              <Icon className="in-data-[type=loading]:animate-spin in-data-[type=error]:text-destructive in-data-[type=info]:text-info in-data-[type=success]:text-success in-data-[type=warning]:text-warning in-data-[type=loading]:opacity-80" />
            </div>
          )}
          <div className="flex min-w-0 flex-col gap-0.5">
            <Toast.Title className="font-medium" data-slot="toast-title" />
            <Toast.Description className="text-muted-foreground" data-slot="toast-description" />
          </div>
        </div>
        {toast.actionProps && (
          <Toast.Action
            className={buttonVariants({ size: 'xs' })}
            data-slot="toast-action"
            onClick={toast.actionProps.onClick}
          >
            {toast.actionProps.children}
          </Toast.Action>
        )}
      </Toast.Content>
      <Toast.Close className="rounded p-0.5 opacity-50 hover:opacity-100 hover:bg-muted transition-opacity">
        <X className="h-3.5 w-3.5" />
      </Toast.Close>
    </Toast.Root>
  )
}

function SidebarToastOverlayInner({ sidebarOpen, sidebarSide }: SidebarToastOverlayProps) {
  const { toasts } = Toast.useToastManager()
  const swipeDirection: ('left' | 'right' | 'down')[] =
    sidebarSide === 'left' ? ['left', 'down'] : ['right', 'down']

  if (sidebarOpen) {
    const visibleToasts = toasts.slice(0, 3)
    return (
      <Toast.Viewport
        className="pointer-events-none absolute inset-x-0 bottom-0 z-50 flex w-full flex-col-reverse gap-2 p-2"
        data-slot="sidebar-toast-viewport"
      >
        {visibleToasts.map((toast) => (
          <SidebarToastItem key={toast.id} toast={toast} swipeDirection={swipeDirection} />
        ))}
      </Toast.Viewport>
    )
  }

  const fixedSideClass = sidebarSide === 'left' ? 'left-4' : 'right-4'

  return (
    <Toast.Portal>
      <Toast.Viewport
        className={`fixed bottom-4 z-[var(--z-overlay,60)] flex w-72 flex-col gap-2 ${fixedSideClass}`}
        data-slot="sidebar-toast-viewport-fallback"
      >
        {toasts.map((toast) => (
          <SidebarToastItem key={toast.id} toast={toast} swipeDirection={swipeDirection} />
        ))}
      </Toast.Viewport>
    </Toast.Portal>
  )
}

export function SidebarToastOverlay({ sidebarOpen, sidebarSide }: SidebarToastOverlayProps) {
  return (
    <Toast.Provider toastManager={toastManager}>
      <SidebarToastOverlayInner sidebarOpen={sidebarOpen} sidebarSide={sidebarSide} />
    </Toast.Provider>
  )
}
