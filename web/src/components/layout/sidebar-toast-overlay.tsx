import { Toast } from '@base-ui/react/toast'
import {
  CircleAlertIcon,
  CircleCheckIcon,
  InfoIcon,
  LoaderCircleIcon,
  TriangleAlertIcon,
  X,
} from 'lucide-react'
import { buttonVariants } from '@/components/ui/button-variants'
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

type SidebarToast = ReturnType<typeof Toast.useToastManager>['toasts'][number]

/** How many toasts the sidebar's narrow column shows at once. */
const SIDEBAR_TOAST_LIMIT = 3

/**
 * A toast with `timeout: 0` opted out of auto-dismiss (`toast.show({duration: 0})`),
 * so nothing but the condition clearing will ever remove it — ConnectionIndicator's
 * "Backend unavailable" is the one that matters.
 */
function isPinned(toast: SidebarToast): boolean {
  return toast.timeout === 0
}

/**
 * Window the list WITHOUT ever dropping a pinned toast. base-ui inserts
 * newest-first, so the old `toasts.slice(0, 3)` kept the three newest and
 * discarded the rest: an outage that also produced three failure toasts evicted
 * the only thing on screen saying the backend was down, and — being
 * undismissable-but-also-unrenderable — nothing could bring it back.
 *
 * Pinned toasts are all kept; transient ones fill whatever slots are left, still
 * newest-first, so the oldest transient is what gets dropped.
 */
function selectVisibleToasts(toasts: SidebarToast[]): SidebarToast[] {
  const pinnedCount = toasts.reduce((count, toast) => count + (isPinned(toast) ? 1 : 0), 0)
  let transientSlots = Math.max(0, SIDEBAR_TOAST_LIMIT - pinnedCount)

  const visible: SidebarToast[] = []
  for (const toast of toasts) {
    if (isPinned(toast)) {
      visible.push(toast)
      continue
    }
    if (transientSlots > 0) {
      transientSlots -= 1
      visible.push(toast)
    }
  }
  return visible
}

function SidebarToastItem({
  toast,
  swipeDirection,
}: {
  toast: SidebarToast
  swipeDirection: ('left' | 'right' | 'down')[]
}) {
  const Icon = toast.type ? TOAST_ICONS[toast.type as keyof typeof TOAST_ICONS] : null

  return (
    <Toast.Root
      toast={toast}
      swipeDirection={swipeDirection}
      className="pointer-events-auto relative w-full overflow-hidden rounded-lg border bg-popover px-3.5 py-3 text-sm text-popover-foreground shadow-lg/5 not-dark:bg-clip-padding transition-opacity duration-200 data-starting-style:opacity-0 data-ending-style:opacity-0"
    >
      <Toast.Content className="flex min-w-0 flex-col gap-2">
        <div className="flex min-w-0 gap-2 pr-6">
          {Icon && (
            <div
              className="[&>svg]:h-lh [&>svg]:w-4 [&_svg]:pointer-events-none [&_svg]:shrink-0"
              data-slot="toast-icon"
              // The loading icon's spin comes from the variant class
              // in-data-[type=loading]:animate-spin, which the .animate-spin
              // reduced-motion exemption in index.css can't match — this
              // attribute keeps the spinner (status, not decoration) running
              // under prefers-reduced-motion.
              data-essential-motion=""
            >
              <Icon className="in-data-[type=loading]:animate-spin in-data-[type=error]:text-destructive in-data-[type=info]:text-info in-data-[type=success]:text-success in-data-[type=warning]:text-warning in-data-[type=loading]:opacity-80" />
            </div>
          )}
          <div className="flex min-w-0 flex-col gap-0.5">
            <Toast.Title className="min-w-0 break-words font-medium" data-slot="toast-title" />
            <Toast.Description
              className="min-w-0 break-words text-muted-foreground"
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
      <Toast.Close className="absolute top-3 right-3 rounded p-0.5 opacity-50 hover:opacity-100 hover:bg-muted transition-opacity">
        <X className="h-3.5 w-3.5" />
      </Toast.Close>
    </Toast.Root>
  )
}

// The two `Toast.Viewport`s are anchored (`sidebar-toast-viewport` /
// `-fallback`, reusing this file's own `data-slot` names) so a future Rust
// port has a root to measure — no port exists yet (P3.52 ported the other
// four sidebar fragments and deliberately left this one), so these ids are
// read off this component's own structure, not matched against any Rust
// source (native/oracle/ANCHORS.md's own rule for that case). A
// `SidebarToastItem` is deliberately left unanchored: its count is a live
// toast queue's own size (0..SIDEBAR_TOAST_LIMIT), a property of the cell
// rather than of the surface — the identical reason `select-item` is
// undeclared in `oracleSurfaceScope` (see extract.ts).
function SidebarToastOverlayInner({ sidebarOpen, sidebarSide }: SidebarToastOverlayProps) {
  const { toasts } = Toast.useToastManager()
  const swipeDirection: ('left' | 'right' | 'down')[] =
    sidebarSide === 'left' ? ['left', 'down'] : ['right', 'down']

  if (sidebarOpen) {
    const visibleToasts = selectVisibleToasts(toasts)
    return (
      <Toast.Viewport
        className="pointer-events-none absolute inset-x-0 bottom-0 z-50 flex w-full flex-col-reverse gap-2 p-2"
        data-slot="sidebar-toast-viewport"
        data-oracle-id="sidebar-toast-viewport"
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
        data-oracle-id="sidebar-toast-viewport-fallback"
      >
        {toasts.map((toast) => (
          <SidebarToastItem key={toast.id} toast={toast} swipeDirection={swipeDirection} />
        ))}
      </Toast.Viewport>
    </Toast.Portal>
  )
}

/**
 * The ONE AND ONLY viewport for `toastManager`. Do not add a second
 * `Toast.Provider` for this manager anywhere else in the tree.
 *
 * `createToastManager()` is a stateless emitter — every subscribed provider
 * builds its OWN copy of the toast list, so two providers means every toast
 * renders twice wherever both are mounted. That's what the old root-level
 * `<ToastProvider>` did; its `suppressToasts={ideShellMounted}` guard was a
 * single global boolean flipped by IDEShell's own mount effect, so any
 * remount race (HMR, route transition, unmount-after-mount ordering) left it
 * reading false while IDEShell was up and duplicated every toast.
 *
 * Consequence of being the only viewport: toasts fired while IDEShell is
 * unmounted (e.g. the /oobe route) are dropped, not queued — the manager
 * holds no state of its own.
 */
export function SidebarToastOverlay({ sidebarOpen, sidebarSide }: SidebarToastOverlayProps) {
  return (
    <Toast.Provider toastManager={toastManager}>
      <SidebarToastOverlayInner sidebarOpen={sidebarOpen} sidebarSide={sidebarSide} />
    </Toast.Provider>
  )
}
