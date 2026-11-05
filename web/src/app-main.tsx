import '@/lib/transport/polyfill'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { enableMapSet } from 'immer'
import { MotionConfig } from 'framer-motion'
import { RouterProvider, createRouter, createHashHistory } from '@tanstack/react-router'
import { TooltipProvider } from '@/components/ui/tooltip'
import { PrimitiveDialogProvider } from '@/components/ui/primitive-dialog-service'
import { routeTree } from './routeTree.gen'
import { connectDaemonEvents } from '@/lib/events/connect'
import { initializeSettingsStore } from '@/features/settings/store'
import { startSettingsStorageSync } from '@/features/settings/lib/settings-storage-sync'
import { startHostThemeSync } from '@/features/terminal/lib/host-theme'
import { initializeIconThemes } from '@/extensions/icon-themes/icon-theme-initializer'
import { installPerfObserver, perfEnabled, pushPerfEntry } from '@/lib/perf/instrumentation'
import { prefetchEditorChunks } from '@/features/panes/components/prefetch-editor-chunks'
import { hydrateCriticalStores } from '@/lib/boot'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import { startHostStatus } from '@/features/hosts/lib/host-status'
import { installGlobalReporting, report } from '@/lib/errors/report'
import { startStream } from '@/core/stream/stream'
import { followChatLifecycle } from '@/features/agent/stores/chat-lifecycle'
import '@/core/entities/entity-store'

// First in boot: perf marks are no-ops until the observer exists, and it self-gates via
// perfEnabled(), so it is safe in every build.
installPerfObserver()

// Every rejection nothing handled reaches the user, not only the console.
const uninstallGlobalReporting = installGlobalReporting()
if (import.meta.hot) {
  import.meta.hot.dispose(() => uninstallGlobalReporting())
}

// The one stream, holding the projects scope every route and the sidebar read.
const stopStream = startStream()
const stopFollowingChats = followChatLifecycle()
if (import.meta.hot) {
  import.meta.hot.dispose(() => {
    stopFollowingChats()
    stopStream()
  })
}

// Required for Zustand stores that use immer middleware with Set/Map state
enableMapSet()

// Every paired host's status, for the app's life: its probes and its streams' refusals.
const stopHostStatus = startHostStatus()
if (import.meta.hot) import.meta.hot.dispose(() => stopHostStatus())

// Wire up daemon event listeners for cache invalidation.
// Cleanup is registered with Vite HMR so hot-reloading doesn't leak listeners.
const disconnectDaemonEvents = connectDaemonEvents()
if (import.meta.hot) {
  import.meta.hot.dispose(() => disconnectDaemonEvents())
}

// At boot, not on terminal open: the daemon seeds these colours into each PTY it spawns, and a CLI
// querying the background has asked before any client attaches. The phone has no local daemon.
if (hasLocalDaemon()) {
  const stopHostThemeSync = startHostThemeSync()
  if (import.meta.hot) import.meta.hot.dispose(() => stopHostThemeSync())
}

// Register all built-in icon themes with the registry before React renders
// so the icon theme dropdown in settings is populated when first opened
initializeIconThemes()

// Kick off settings load. The store starts with defaults and updates
// asynchronously when localStorage values are loaded — this fires the
// subscriptions in settings-store.ts which propagate to editor/theme/etc.
void initializeSettingsStore()

// The phone's settings sheet is another webview saving to the same localStorage.
const stopSettingsStorageSync = startSettingsStorageSync()
if (import.meta.hot) import.meta.hot.dispose(() => stopSettingsStorageSync())

// Once startup settles, so neither first file-open nor cold launch pays for the editor chunks;
// here rather than in pane-container module scope so it runs exactly once.
prefetchEditorChunks()

// Native window blur is applied on the Rust side via window-vibrancy (see lib.rs).

// react-scan is opt-in even in dev (it costs about half of dev-mode performance):
//   localStorage.setItem('crowbar:react-scan', '1') then reload.
if (import.meta.env.DEV && localStorage.getItem('crowbar:react-scan') === '1') {
  void import('react-scan').then(({ scan }) => scan({ enabled: true, log: false }))
}

// Report INP (Interaction to Next Paint) into the shared perf ring buffer
// whenever perf instrumentation is armed (dev, or __CROWBAR_PERF__ in prod).
if (perfEnabled()) {
  void import('web-vitals').then(({ onINP }) => {
    onINP(
      (m) => {
        pushPerfEntry({
          name: `INP:${m.rating}`,
          startTime: m.entries[0]?.startTime ?? 0,
          duration: m.value,
          entryType: 'inp',
        })
      },
      { reportAllChanges: true },
    )
  })
}

const router = createRouter({ routeTree, history: createHashHistory() })

function renderApp() {
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      {/* One global gate defers every framer-motion animation to the OS prefers-reduced-motion
          (WCAG 2.3.3) instead of threading useReducedMotion() through each. */}
      <MotionConfig reducedMotion="user">
        <TooltipProvider>
          {/* primitiveConfirm/alert/prompt render through this host; without it
              every primitive dialog request queues forever and callers (terminal
              link confirm, multi-line paste guard) look silently dead. */}
          <PrimitiveDialogProvider>
            <RouterProvider router={router} />
          </PrimitiveDialogProvider>
        </TooltipProvider>
      </MotionConfig>
    </StrictMode>,
  )
}

hydrateCriticalStores()
  .catch((err: unknown) => report(err, { action: 'Restore the last session' }))
  .finally(renderApp)
