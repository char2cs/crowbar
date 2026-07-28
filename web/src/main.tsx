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
import { ensureStartupAppearanceApplied } from '@/features/settings/lib/appearance-bootstrap'
import { initializeIconThemes } from '@/extensions/icon-themes/icon-theme-initializer'
import { initTreeCacheSubscription } from '@/features/editor/stores/tree-cache-store'
import { initViewStoreSubscription } from '@/features/editor/stores/view-store'
import { installPerfObserver, perfEnabled, pushPerfEntry } from '@/lib/perf/instrumentation'
import { prefetchEditorChunks } from '@/features/panes/components/prefetch-editor-chunks'
import './index.css'

// Must run before anything else in boot: markStart/markEnd calls elsewhere
// (invoked from the init calls below or their imports) are no-ops until the
// observer is installed, but once installed it self-gates via perfEnabled()
// so this is a safe unconditional call in every build.
installPerfObserver()

// Required for Zustand stores that use immer middleware with Set/Map state
enableMapSet()

// Wire up daemon event listeners for cache invalidation.
// Cleanup is registered with Vite HMR so hot-reloading doesn't leak listeners.
const disconnectDaemonEvents = connectDaemonEvents()
if (import.meta.hot) {
  import.meta.hot.dispose(() => disconnectDaemonEvents())
}

// Apply the cached theme immediately (synchronous) so the correct dark/light
// class is set before React renders anything — prevents a flash of light mode.
ensureStartupAppearanceApplied()

// Register all built-in icon themes with the registry before React renders
// so the icon theme dropdown in settings is populated when first opened
initializeIconThemes()

// Kick off settings load. The store starts with defaults and updates
// asynchronously when localStorage values are loaded — this fires the
// subscriptions in settings-store.ts which propagate to editor/theme/etc.
void initializeSettingsStore()

// Wire up tree-sitter cache cleanup: removes parse trees when buffers are closed.
initTreeCacheSubscription()
initViewStoreSubscription()

// Prefetch the editor/diff pane chunks once startup settles (spec P1): first
// file-open should not pay the network/parse cost, but cold launch must not
// either. Fired here (not as a pane-container module-scope side effect) so it
// runs exactly once, after the rest of boot is wired up.
prefetchEditorChunks()

// Native window blur is applied on the Rust side via window-vibrancy (see lib.rs).

// Render-tracking overlay, OPT-IN even in dev: react-scan instruments every
// render and costs roughly half of the apparent dev-mode performance, so it
// must never run during feel-testing. Arm it only for render investigations:
//   localStorage.setItem('crowbar:react-scan', '1')  (then reload; remove to disarm)
// The import.meta.env.DEV guard still keeps it out of the production bundle.
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
      {/* reducedMotion="user" makes every framer-motion animation in the app
          (the m and motion component families, LazyMotion or not) defer to
          the OS-level prefers-reduced-motion setting instead of always
          animating — WCAG 2.3.3. One global gate here covers every current
          and future consumer instead of threading useReducedMotion()
          through each. */}
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

// In mock mode, wait for MSW to register its service worker before rendering
// so that all API calls from keepMounted components are intercepted.
// Use finally so a failed MSW startup still renders the app.
if (import.meta.env.VITE_USE_MOCK === 'true') {
  import('./mocks/browser')
    .then(({ worker }) => worker.start({ onUnhandledRequest: 'warn' }))
    .catch(console.error)
    .finally(renderApp)
} else {
  renderApp()
}
