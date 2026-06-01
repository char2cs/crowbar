import '@/lib/transport/polyfill'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { enableMapSet } from 'immer'
import { RouterProvider, createRouter, createHashHistory } from '@tanstack/react-router'
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'
import { TooltipProvider } from '@/components/ui/tooltip'
import { routeTree } from './routeTree.gen'
import { queryClient } from '@/lib/queries/client'
import { connectDaemonEvents } from '@/lib/events/connect'
import { initializeSettingsStore } from '@/features/settings/store'
import { ensureStartupAppearanceApplied } from '@/features/settings/lib/appearance-bootstrap'
import { initializeIconThemes } from '@/extensions/icon-themes/icon-theme-initializer'
import { initTreeCacheSubscription } from '@/features/editor/stores/tree-cache-store'
import { initViewStoreSubscription } from '@/features/editor/stores/view-store'
import { persister } from '@/lib/persistence/query-persister'
import './index.css'

// Required for Zustand stores that use immer middleware with Set/Map state
enableMapSet()

// Wire up daemon event listeners for cache invalidation.
// Cleanup is registered with Vite HMR so hot-reloading doesn't leak listeners.
const disconnectDaemonEvents = connectDaemonEvents(queryClient)
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

const router = createRouter({ routeTree, history: createHashHistory() })

// Fold the active mock scenario into the React Query persistence buster. When the
// scenario changes, the buster changes, so PersistQueryClientProvider DISCARDS the
// entire persisted query-cache on the next load. This is what makes scenario
// switching deterministic: without it, the old in-memory cache can be flushed back
// to IDB during the reload's unload (racing applyScenario's manual clear), and the
// restored stale data would be served to useQuery consumers — the app would
// intermittently "not apply" the new scenario. In real-backend mode (no mock) the
// scenario is irrelevant, so the buster stays as just the app version.
function scenarioBusterSuffix(): string {
  if (import.meta.env.VITE_USE_MOCK !== 'true') return ''
  try {
    const raw = localStorage.getItem('crowbar.chaos')
    const scenario = raw ? JSON.parse(raw)?.state?.scenario : null
    return scenario ? `-${scenario}` : ''
  } catch {
    return ''
  }
}

const queryCacheBuster = `${__APP_VERSION__}${scenarioBusterSuffix()}`

function renderApp() {
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <PersistQueryClientProvider
        client={queryClient}
        persistOptions={{
          persister,
          maxAge: 7 * 24 * 60 * 60 * 1000,
          buster: queryCacheBuster,
        }}
      >
        <TooltipProvider>
          <RouterProvider router={router} />
        </TooltipProvider>
      </PersistQueryClientProvider>
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
